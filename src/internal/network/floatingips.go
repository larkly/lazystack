package network

import (
	"cmp"
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/larkly/lazystack/internal/shared"
)

// FloatingIP is a simplified floating IP representation.
type FloatingIP struct {
	ID                string
	FloatingIP        string
	FixedIP           string
	FloatingNetworkID string
	PortID            string
	TenantID          string
	Status            string
	RouterID          string
}

// ListFloatingIPs fetches all floating IPs.
func ListFloatingIPs(ctx context.Context, client *gophercloud.ServiceClient) ([]FloatingIP, error) {
	shared.Debugf("[network] listing floating IPs")
	var result []FloatingIP
	err := floatingips.List(client, floatingips.ListOpts{}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := floatingips.ExtractFloatingIPs(page)
		if err != nil {
			return false, err
		}
		for _, fip := range extracted {
			// Newer Neutron releases may report the owner only as project_id.
			owner := fip.TenantID
			if owner == "" {
				owner = fip.ProjectID
			}
			result = append(result, FloatingIP{
				ID:                fip.ID,
				FloatingIP:        fip.FloatingIP,
				FixedIP:           fip.FixedIP,
				FloatingNetworkID: fip.FloatingNetworkID,
				PortID:            fip.PortID,
				TenantID:          owner,
				Status:            fip.Status,
				RouterID:          fip.RouterID,
			})
		}
		return true, nil
	})
	if err != nil {
		shared.Debugf("[network] list floating IPs: %v", err)
		return nil, fmt.Errorf("listing floating IPs: %w", err)
	}
	shared.Debugf("[network] listed %d floating IPs", len(result))
	return result, nil
}

// AllocateFloatingIP allocates a new floating IP from the given external network.
func AllocateFloatingIP(ctx context.Context, client *gophercloud.ServiceClient, networkID string) (*FloatingIP, error) {
	shared.Debugf("[network] allocating floating IP from network %s", networkID)
	r := floatingips.Create(ctx, client, floatingips.CreateOpts{
		FloatingNetworkID: networkID,
	})
	fip, err := r.Extract()
	if err != nil {
		shared.Debugf("[network] allocate floating IP from network %s: %v", networkID, err)
		return nil, fmt.Errorf("allocating floating IP: %w", err)
	}
	shared.Debugf("[network] allocated floating IP %s (ID: %s)", fip.FloatingIP, fip.ID)
	return &FloatingIP{
		ID:                fip.ID,
		FloatingIP:        fip.FloatingIP,
		FloatingNetworkID: fip.FloatingNetworkID,
		Status:            fip.Status,
	}, nil
}

// AssociateFloatingIP associates a floating IP with a port. An empty port ID
// is rejected: it would be sent as port_id:null and silently disassociate
// the floating IP instead (use DisassociateFloatingIP for that).
func AssociateFloatingIP(ctx context.Context, client *gophercloud.ServiceClient, fipID, portID string) error {
	return AssociateFloatingIPToAddress(ctx, client, fipID, portID, "")
}

// AssociateFloatingIPToAddress associates a floating IP with a specific
// fixed IP on a port. fixedIP may be empty when the port has a single IPv4
// address; Neutron requires it when the port has several.
func AssociateFloatingIPToAddress(ctx context.Context, client *gophercloud.ServiceClient, fipID, portID, fixedIP string) error {
	shared.Debugf("[network] associating floating IP %s with port %s (fixed IP: %q)", fipID, portID, fixedIP)
	if strings.TrimSpace(portID) == "" {
		return fmt.Errorf("associating floating IP %s: no port selected", fipID)
	}
	_, err := floatingips.Update(ctx, client, fipID, floatingips.UpdateOpts{
		PortID:  &portID,
		FixedIP: fixedIP,
	}).Extract()
	if err != nil {
		shared.Debugf("[network] associate floating IP %s with port %s: %v", fipID, portID, err)
		return fmt.Errorf("associating floating IP %s: %w", fipID, err)
	}
	shared.Debugf("[network] associated floating IP %s with port %s", fipID, portID)
	return nil
}

// DisassociateFloatingIP removes a floating IP from its port.
func DisassociateFloatingIP(ctx context.Context, client *gophercloud.ServiceClient, fipID string) error {
	shared.Debugf("[network] disassociating floating IP %s", fipID)
	empty := ""
	_, err := floatingips.Update(ctx, client, fipID, floatingips.UpdateOpts{
		PortID: &empty,
	}).Extract()
	if err != nil {
		shared.Debugf("[network] disassociate floating IP %s: %v", fipID, err)
		return fmt.Errorf("disassociating floating IP %s: %w", fipID, err)
	}
	shared.Debugf("[network] disassociated floating IP %s", fipID)
	return nil
}

// ReleaseFloatingIP deletes a floating IP.
func ReleaseFloatingIP(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[network] releasing floating IP %s", id)
	r := floatingips.Delete(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[network] release floating IP %s: %v", id, r.Err)
		return fmt.Errorf("releasing floating IP %s: %w", id, r.Err)
	}
	shared.Debugf("[network] released floating IP %s", id)
	return nil
}

// FloatingIPTarget is an IPv4 fixed address on a server port that a
// floating IP (IPv4 NAT) can be bound to.
type FloatingIPTarget struct {
	PortID    string
	PortName  string
	NetworkID string
	SubnetID  string
	IPAddress string
	// ExternalNetworkIDs are the external networks whose routers have an
	// interface on SubnetID, i.e. the networks a floating IP must come from
	// to reach this address.
	ExternalNetworkIDs []string
}

// ReachableFrom reports whether a router connects the target's subnet to the
// given external network.
func (t FloatingIPTarget) ReachableFrom(externalNetworkID string) bool {
	return slices.Contains(t.ExternalNetworkIDs, externalNetworkID)
}

// Label is a short human-readable description of the target.
func (t FloatingIPTarget) Label() string {
	name := t.PortName
	if name == "" {
		name = shared.ShortID(t.PortID)
	}
	return t.IPAddress + " (port " + name + ")"
}

// ListFloatingIPTargets lists the IPv4 fixed addresses of a server's ports,
// annotated with the external networks routed to each address's subnet. The
// result is sorted by port name, port ID and address so callers can present
// it deterministically. Router discovery failures are logged and leave
// ExternalNetworkIDs empty rather than failing the lookup.
func ListFloatingIPTargets(ctx context.Context, client *gophercloud.ServiceClient, serverID string) ([]FloatingIPTarget, error) {
	shared.Debugf("[network] listing floating IP targets for server %s", serverID)
	serverPorts, err := ListPortsByDevice(ctx, client, serverID)
	if err != nil {
		return nil, err
	}
	var targets []FloatingIPTarget
	for _, p := range serverPorts {
		for _, ip := range p.FixedIPs {
			addr, err := netip.ParseAddr(ip.IPAddress)
			if err != nil || !addr.Unmap().Is4() {
				continue
			}
			targets = append(targets, FloatingIPTarget{
				PortID:    p.ID,
				PortName:  p.Name,
				NetworkID: p.NetworkID,
				SubnetID:  ip.SubnetID,
				IPAddress: ip.IPAddress,
			})
		}
	}
	if len(targets) == 0 {
		return nil, nil
	}

	// Map each subnet to the external networks of routers attached to it.
	subnetExt := make(map[string][]string)
	routerList, err := ListRouters(ctx, client)
	if err != nil {
		shared.Debugf("[network] floating IP targets: cannot list routers: %v", err)
	}
	for _, r := range routerList {
		if r.ExternalGatewayNetworkID == "" {
			continue
		}
		ifaces, err := ListRouterInterfaces(ctx, client, r.ID)
		if err != nil {
			shared.Debugf("[network] floating IP targets: cannot list interfaces of router %s: %v", r.ID, err)
			continue
		}
		for _, iface := range ifaces {
			if !slices.Contains(subnetExt[iface.SubnetID], r.ExternalGatewayNetworkID) {
				subnetExt[iface.SubnetID] = append(subnetExt[iface.SubnetID], r.ExternalGatewayNetworkID)
			}
		}
	}
	for i := range targets {
		targets[i].ExternalNetworkIDs = subnetExt[targets[i].SubnetID]
	}
	slices.SortStableFunc(targets, func(a, b FloatingIPTarget) int {
		return cmp.Or(
			cmp.Compare(a.PortName, b.PortName),
			cmp.Compare(a.PortID, b.PortID),
			cmp.Compare(a.IPAddress, b.IPAddress),
		)
	})
	shared.Debugf("[network] found %d floating IP targets for server %s", len(targets), serverID)
	return targets, nil
}
