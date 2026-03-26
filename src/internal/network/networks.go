package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/subnets"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// Network is a simplified representation of a Neutron network.
type Network struct {
	ID        string
	Name      string
	Status    string
	Shared    bool
	External  bool
	SubnetIDs []string
}

// Subnet is a simplified representation of a Neutron subnet.
type Subnet struct {
	ID              string
	Name            string
	NetworkID       string
	CIDR            string
	GatewayIP       string
	IPVersion       int
	EnableDHCP      bool
	AllocationPools []AllocationPool
}

// AllocationPool is a DHCP allocation pool range.
type AllocationPool struct {
	Start string
	End   string
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
				ID:        n.ID,
				Name:      n.Name,
				Status:    n.Status,
				Shared:    n.Shared,
				SubnetIDs: n.Subnets,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing networks: %w", err)
	}
	return result, nil
}

// ListSubnets fetches all subnets.
func ListSubnets(ctx context.Context, client *gophercloud.ServiceClient) ([]Subnet, error) {
	var result []Subnet
	err := subnets.List(client, subnets.ListOpts{}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := subnets.ExtractSubnets(page)
		if err != nil {
			return false, err
		}
		for _, s := range extracted {
			sub := Subnet{
				ID:         s.ID,
				Name:       s.Name,
				NetworkID:  s.NetworkID,
				CIDR:       s.CIDR,
				GatewayIP:  s.GatewayIP,
				IPVersion:  s.IPVersion,
				EnableDHCP: s.EnableDHCP,
			}
			for _, pool := range s.AllocationPools {
				sub.AllocationPools = append(sub.AllocationPools, AllocationPool{
					Start: pool.Start,
					End:   pool.End,
				})
			}
			result = append(result, sub)
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing subnets: %w", err)
	}
	return result, nil
}

// ListExternalNetworks fetches networks where router:external is true.
func ListExternalNetworks(ctx context.Context, client *gophercloud.ServiceClient) ([]Network, error) {
	url := client.ServiceURL("networks") + "?router:external=true"
	var body struct {
		Networks []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"networks"`
	}
	resp, err := client.Get(ctx, url, &body, nil)
	if err != nil {
		return nil, fmt.Errorf("listing external networks: %w", err)
	}
	resp.Body.Close()

	result := make([]Network, len(body.Networks))
	for i, n := range body.Networks {
		result[i] = Network{ID: n.ID, Name: n.Name, Status: n.Status}
	}
	return result, nil
}

// FindServerPortID returns the first port ID for the given server (device_id).
func FindServerPortID(ctx context.Context, client *gophercloud.ServiceClient, serverID string) (string, error) {
	var portID string
	err := ports.List(client, ports.ListOpts{DeviceID: serverID}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := ports.ExtractPorts(page)
		if err != nil {
			return false, err
		}
		if len(extracted) > 0 {
			portID = extracted[0].ID
			return false, nil // stop after first
		}
		return true, nil
	})
	if err != nil {
		return "", fmt.Errorf("finding port for server %s: %w", serverID, err)
	}
	if portID == "" {
		return "", fmt.Errorf("no ports found for server %s", serverID)
	}
	return portID, nil
}
