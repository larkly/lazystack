package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/portsecurity"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// Port is a simplified representation of a Neutron port.
type Port struct {
	ID                  string
	Name                string
	Description         string
	Status              string
	MACAddress          string
	FixedIPs            []FixedIP
	DeviceOwner         string
	DeviceID            string
	NetworkID           string
	SecurityGroups      []string
	AllowedAddressPairs []AddressPair
	AdminStateUp        bool
	PortSecurityEnabled bool
}

// FixedIP is an IP address assigned to a port.
type FixedIP struct {
	SubnetID  string
	IPAddress string
}

// AddressPair is an allowed address pair on a port.
type AddressPair struct {
	IPAddress  string
	MACAddress string
}

// portWithExt is used to extract port security from API responses.
type portWithExt struct {
	ports.Port
	portsecurity.PortSecurityExt
}

// mapPort converts a portWithExt to our simplified Port.
func mapPort(p portWithExt) Port {
	port := Port{
		ID:                  p.ID,
		Name:                p.Name,
		Description:         p.Description,
		Status:              p.Status,
		MACAddress:          p.MACAddress,
		DeviceOwner:         p.DeviceOwner,
		DeviceID:            p.DeviceID,
		NetworkID:           p.NetworkID,
		SecurityGroups:      p.SecurityGroups,
		AdminStateUp:        p.AdminStateUp,
		PortSecurityEnabled: p.PortSecurityExt.PortSecurityEnabled,
	}
	for _, ip := range p.FixedIPs {
		port.FixedIPs = append(port.FixedIPs, FixedIP{
			SubnetID:  ip.SubnetID,
			IPAddress: ip.IPAddress,
		})
	}
	for _, ap := range p.AllowedAddressPairs {
		port.AllowedAddressPairs = append(port.AllowedAddressPairs, AddressPair{
			IPAddress:  ap.IPAddress,
			MACAddress: ap.MACAddress,
		})
	}
	return port
}

// mapPortBasic converts a basic ports.Port (without extension) to our Port.
func mapPortBasic(p ports.Port) Port {
	port := Port{
		ID:             p.ID,
		Name:           p.Name,
		Description:    p.Description,
		Status:         p.Status,
		MACAddress:     p.MACAddress,
		DeviceOwner:    p.DeviceOwner,
		DeviceID:       p.DeviceID,
		NetworkID:      p.NetworkID,
		SecurityGroups: p.SecurityGroups,
		AdminStateUp:   p.AdminStateUp,
	}
	for _, ip := range p.FixedIPs {
		port.FixedIPs = append(port.FixedIPs, FixedIP{
			SubnetID:  ip.SubnetID,
			IPAddress: ip.IPAddress,
		})
	}
	for _, ap := range p.AllowedAddressPairs {
		port.AllowedAddressPairs = append(port.AllowedAddressPairs, AddressPair{
			IPAddress:  ap.IPAddress,
			MACAddress: ap.MACAddress,
		})
	}
	return port
}

// extractPortsWithExt extracts ports from a page with the portsecurity extension.
func extractPortsWithExt(page pagination.Page) ([]portWithExt, error) {
	var s []portWithExt
	err := ports.ExtractPortsInto(page, &s)
	return s, err
}

// GetPort fetches a single port by ID.
func GetPort(ctx context.Context, client *gophercloud.ServiceClient, portID string) (*Port, error) {
	var result portWithExt
	err := ports.Get(ctx, client, portID).ExtractInto(&result)
	if err != nil {
		return nil, fmt.Errorf("getting port %s: %w", portID, err)
	}
	port := mapPort(result)
	return &port, nil
}

// FindRouterPortOnNetwork returns the router interface port on a given network, if any.
func FindRouterPortOnNetwork(ctx context.Context, client *gophercloud.ServiceClient, routerID, networkID string) (*Port, error) {
	var result *Port
	err := ports.List(client, ports.ListOpts{
		DeviceID:    routerID,
		DeviceOwner: "network:router_interface",
		NetworkID:   networkID,
	}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := ports.ExtractPorts(page)
		if err != nil {
			return false, err
		}
		if len(extracted) > 0 {
			p := mapPortBasic(extracted[0])
			result = &p
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("finding router port on network %s: %w", networkID, err)
	}
	return result, nil
}

// AddFixedIPToPort adds a fixed IP to an existing port.
func AddFixedIPToPort(ctx context.Context, client *gophercloud.ServiceClient, portID string, existing []FixedIP, subnetID, ipAddress string) error {
	fixedIPs := make([]ports.IP, 0, len(existing)+1)
	for _, ip := range existing {
		fixedIPs = append(fixedIPs, ports.IP{
			SubnetID:  ip.SubnetID,
			IPAddress: ip.IPAddress,
		})
	}
	newIP := ports.IP{SubnetID: subnetID}
	if ipAddress != "" {
		newIP.IPAddress = ipAddress
	}
	fixedIPs = append(fixedIPs, newIP)

	_, err := ports.Update(ctx, client, portID, ports.UpdateOpts{
		FixedIPs: fixedIPs,
	}).Extract()
	if err != nil {
		return fmt.Errorf("adding fixed IP to port %s: %w", portID, err)
	}
	return nil
}

// RemoveFixedIPFromPort removes a single fixed IP (by subnet ID) from a port,
// keeping all other fixed IPs intact.
func RemoveFixedIPFromPort(ctx context.Context, client *gophercloud.ServiceClient, portID, subnetID string) error {
	p, err := ports.Get(ctx, client, portID).Extract()
	if err != nil {
		return fmt.Errorf("getting port %s: %w", portID, err)
	}

	var remaining []ports.IP
	for _, ip := range p.FixedIPs {
		if ip.SubnetID != subnetID {
			remaining = append(remaining, ports.IP{
				SubnetID:  ip.SubnetID,
				IPAddress: ip.IPAddress,
			})
		}
	}

	_, err = ports.Update(ctx, client, portID, ports.UpdateOpts{
		FixedIPs: remaining,
	}).Extract()
	if err != nil {
		return fmt.Errorf("removing fixed IP from port %s: %w", portID, err)
	}
	return nil
}

// ListPortsByDevice fetches all ports attached to a given device (e.g. server).
func ListPortsByDevice(ctx context.Context, client *gophercloud.ServiceClient, deviceID string) ([]Port, error) {
	var result []Port
	err := ports.List(client, ports.ListOpts{DeviceID: deviceID}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := extractPortsWithExt(page)
		if err != nil {
			return false, err
		}
		for _, p := range extracted {
			result = append(result, mapPort(p))
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing ports for device %s: %w", deviceID, err)
	}
	return result, nil
}

// ListPortsBySecurityGroup fetches all ports that have a given security group.
func ListPortsBySecurityGroup(ctx context.Context, client *gophercloud.ServiceClient, sgID string) ([]Port, error) {
	var result []Port
	err := ports.List(client, ports.ListOpts{SecurityGroups: []string{sgID}}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := extractPortsWithExt(page)
		if err != nil {
			return false, err
		}
		for _, p := range extracted {
			result = append(result, mapPort(p))
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing ports for security group %s: %w", sgID, err)
	}
	return result, nil
}

// CreatePort creates a port on the given subnet. If ipAddress is empty,
// Neutron allocates automatically (required for SLAAC/DHCPv6 subnets).
func CreatePort(ctx context.Context, client *gophercloud.ServiceClient, networkID, subnetID, ipAddress string) (*Port, error) {
	fixedIP := ports.IP{SubnetID: subnetID}
	if ipAddress != "" {
		fixedIP.IPAddress = ipAddress
	}
	opts := ports.CreateOpts{
		NetworkID: networkID,
		FixedIPs:  []ports.IP{fixedIP},
	}
	p, err := ports.Create(ctx, client, opts).Extract()
	if err != nil {
		return nil, fmt.Errorf("creating port on network %s: %w", networkID, err)
	}
	port := mapPortBasic(*p)
	return &port, nil
}

// PortCreateOpts holds options for creating a port with full control.
type PortCreateOpts struct {
	NetworkID           string
	Name                string
	Description         string
	FixedIPs            []FixedIP
	SecurityGroups      []string
	AllowedAddressPairs []AddressPair
	AdminStateUp        bool
	PortSecurityEnabled *bool // nil = server default (true)
}

// CreatePortFull creates a port with all available options including port security.
func CreatePortFull(ctx context.Context, client *gophercloud.ServiceClient, opts PortCreateOpts) (*Port, error) {
	baseOpts := ports.CreateOpts{
		NetworkID:    opts.NetworkID,
		Name:         opts.Name,
		Description:  opts.Description,
		AdminStateUp: &opts.AdminStateUp,
	}
	if len(opts.FixedIPs) > 0 {
		fips := make([]ports.IP, len(opts.FixedIPs))
		for i, ip := range opts.FixedIPs {
			fips[i] = ports.IP{SubnetID: ip.SubnetID, IPAddress: ip.IPAddress}
		}
		baseOpts.FixedIPs = fips
	}
	if opts.SecurityGroups != nil {
		baseOpts.SecurityGroups = &opts.SecurityGroups
	}
	if len(opts.AllowedAddressPairs) > 0 {
		pairs := make([]ports.AddressPair, len(opts.AllowedAddressPairs))
		for i, ap := range opts.AllowedAddressPairs {
			pairs[i] = ports.AddressPair{IPAddress: ap.IPAddress, MACAddress: ap.MACAddress}
		}
		baseOpts.AllowedAddressPairs = pairs
	}

	createOpts := portsecurity.PortCreateOptsExt{
		CreateOptsBuilder:   baseOpts,
		PortSecurityEnabled: opts.PortSecurityEnabled,
	}

	var result portWithExt
	err := ports.Create(ctx, client, createOpts).ExtractInto(&result)
	if err != nil {
		return nil, fmt.Errorf("creating port on network %s: %w", opts.NetworkID, err)
	}
	port := mapPort(result)
	return &port, nil
}

// PortUpdateOpts holds options for updating a port.
type PortUpdateOpts struct {
	Name                *string
	Description         *string
	SecurityGroups      *[]string
	AllowedAddressPairs *[]AddressPair
	AdminStateUp        *bool
	PortSecurityEnabled *bool
}

// UpdatePort updates a port with the given options.
func UpdatePort(ctx context.Context, client *gophercloud.ServiceClient, portID string, opts PortUpdateOpts) error {
	baseOpts := ports.UpdateOpts{}
	if opts.Name != nil {
		baseOpts.Name = opts.Name
	}
	if opts.Description != nil {
		baseOpts.Description = opts.Description
	}
	if opts.AdminStateUp != nil {
		baseOpts.AdminStateUp = opts.AdminStateUp
	}
	if opts.SecurityGroups != nil {
		baseOpts.SecurityGroups = opts.SecurityGroups
	}
	if opts.AllowedAddressPairs != nil {
		pairs := make([]ports.AddressPair, len(*opts.AllowedAddressPairs))
		for i, ap := range *opts.AllowedAddressPairs {
			pairs[i] = ports.AddressPair{IPAddress: ap.IPAddress, MACAddress: ap.MACAddress}
		}
		baseOpts.AllowedAddressPairs = &pairs
	}

	updateOpts := portsecurity.PortUpdateOptsExt{
		UpdateOptsBuilder:   baseOpts,
		PortSecurityEnabled: opts.PortSecurityEnabled,
	}

	_, err := ports.Update(ctx, client, portID, updateOpts).Extract()
	if err != nil {
		return fmt.Errorf("updating port %s: %w", portID, err)
	}
	return nil
}

// DeletePort deletes a port by ID.
func DeletePort(ctx context.Context, client *gophercloud.ServiceClient, portID string) error {
	r := ports.Delete(ctx, client, portID)
	if r.Err != nil {
		return fmt.Errorf("deleting port %s: %w", portID, r.Err)
	}
	return nil
}

// ListPorts fetches all ports for a given network.
func ListPorts(ctx context.Context, client *gophercloud.ServiceClient, networkID string) ([]Port, error) {
	var result []Port
	err := ports.List(client, ports.ListOpts{NetworkID: networkID}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := extractPortsWithExt(page)
		if err != nil {
			return false, err
		}
		for _, p := range extracted {
			result = append(result, mapPort(p))
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing ports for network %s: %w", networkID, err)
	}
	return result, nil
}
