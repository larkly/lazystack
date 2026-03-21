package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// Network is a simplified representation of a Neutron network.
type Network struct {
	ID     string
	Name   string
	Status string
	Shared bool
}

// ListNetworks fetches all available networks.
func ListNetworks(ctx context.Context, client *gophercloud.ServiceClient) ([]Network, error) {
	opts := networks.ListOpts{}

	var result []Network
	err := networks.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := networks.ExtractNetworks(page)
		if err != nil {
			return false, err
		}
		for _, n := range extracted {
			result = append(result, Network{
				ID:     n.ID,
				Name:   n.Name,
				Status: n.Status,
				Shared: n.Shared,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing networks: %w", err)
	}
	return result, nil
}
