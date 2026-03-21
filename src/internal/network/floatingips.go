package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/gophercloud/gophercloud/v2/pagination"
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
		return nil, fmt.Errorf("listing floating IPs: %w", err)
	}
	return result, nil
}

// AllocateFloatingIP allocates a new floating IP from the given external network.
func AllocateFloatingIP(ctx context.Context, client *gophercloud.ServiceClient, networkID string) (*FloatingIP, error) {
	r := floatingips.Create(ctx, client, floatingips.CreateOpts{
		FloatingNetworkID: networkID,
	})
	fip, err := r.Extract()
	if err != nil {
		return nil, fmt.Errorf("allocating floating IP: %w", err)
	}
	return &FloatingIP{
		ID:                fip.ID,
		FloatingIP:        fip.FloatingIP,
		FloatingNetworkID: fip.FloatingNetworkID,
		Status:            fip.Status,
	}, nil
}

// AssociateFloatingIP associates a floating IP with a port.
func AssociateFloatingIP(ctx context.Context, client *gophercloud.ServiceClient, fipID, portID string) error {
	_, err := floatingips.Update(ctx, client, fipID, floatingips.UpdateOpts{
		PortID: &portID,
	}).Extract()
	if err != nil {
		return fmt.Errorf("associating floating IP %s: %w", fipID, err)
	}
	return nil
}

// DisassociateFloatingIP removes a floating IP from its port.
func DisassociateFloatingIP(ctx context.Context, client *gophercloud.ServiceClient, fipID string) error {
	empty := ""
	_, err := floatingips.Update(ctx, client, fipID, floatingips.UpdateOpts{
		PortID: &empty,
	}).Extract()
	if err != nil {
		return fmt.Errorf("disassociating floating IP %s: %w", fipID, err)
	}
	return nil
}

// ReleaseFloatingIP deletes a floating IP.
func ReleaseFloatingIP(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := floatingips.Delete(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("releasing floating IP %s: %w", id, r.Err)
	}
	return nil
}
