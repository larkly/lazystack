package network

import (
	"context"
	"fmt"

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
			result = append(result, FloatingIP{
				ID:                fip.ID,
				FloatingIP:        fip.FloatingIP,
				FixedIP:           fip.FixedIP,
				FloatingNetworkID: fip.FloatingNetworkID,
				PortID:            fip.PortID,
				TenantID:          fip.TenantID,
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

// AssociateFloatingIP associates a floating IP with a port.
func AssociateFloatingIP(ctx context.Context, client *gophercloud.ServiceClient, fipID, portID string) error {
	shared.Debugf("[network] associating floating IP %s with port %s", fipID, portID)
	_, err := floatingips.Update(ctx, client, fipID, floatingips.UpdateOpts{
		PortID: &portID,
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
