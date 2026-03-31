package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/subnetpools"
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
	IPv6AddressMode string
	AllocationPools []AllocationPool
}

// AllocationPool is a DHCP allocation pool range.
type AllocationPool struct {
	Start string
	End   string
}

// SubnetPool is a simplified representation of a Neutron subnet pool.
type SubnetPool struct {
	ID               string
	Name             string
	Prefixes         []string
	IPVersion        int
	DefaultPrefixLen int
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
				ID:              s.ID,
				Name:            s.Name,
				NetworkID:       s.NetworkID,
				CIDR:            s.CIDR,
				GatewayIP:       s.GatewayIP,
				IPVersion:       s.IPVersion,
				EnableDHCP:      s.EnableDHCP,
				IPv6AddressMode: s.IPv6AddressMode,
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

// ListSubnetPools fetches all subnet pools.
func ListSubnetPools(ctx context.Context, client *gophercloud.ServiceClient) ([]SubnetPool, error) {
	var result []SubnetPool
	err := subnetpools.List(client, subnetpools.ListOpts{}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := subnetpools.ExtractSubnetPools(page)
		if err != nil {
			return false, err
		}
		for _, sp := range extracted {
			result = append(result, SubnetPool{
				ID:               sp.ID,
				Name:             sp.Name,
				Prefixes:         sp.Prefixes,
				IPVersion:        sp.IPversion,
				DefaultPrefixLen: sp.DefaultPrefixLen,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing subnet pools: %w", err)
	}
	return result, nil
}

// CreateNetwork creates a new network.
func CreateNetwork(ctx context.Context, client *gophercloud.ServiceClient, name string, adminStateUp bool) (*Network, error) {
	r := networks.Create(ctx, client, networks.CreateOpts{
		Name:         name,
		AdminStateUp: &adminStateUp,
	})
	n, err := r.Extract()
	if err != nil {
		return nil, fmt.Errorf("creating network: %w", err)
	}
	return &Network{
		ID:     n.ID,
		Name:   n.Name,
		Status: n.Status,
		Shared: n.Shared,
	}, nil
}

// DeleteNetwork deletes a network.
func DeleteNetwork(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := networks.Delete(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("deleting network %s: %w", id, r.Err)
	}
	return nil
}

// SubnetCreateOpts holds options for creating a subnet.
type SubnetCreateOpts struct {
	NetworkID       string
	Name            string
	CIDR            string
	IPVersion       int
	GatewayIP       string
	EnableDHCP      bool
	SubnetPoolID    string
	PrefixLen       int
	IPv6AddressMode string
	IPv6RAMode      string
}

// CreateSubnet creates a new subnet.
func CreateSubnet(ctx context.Context, client *gophercloud.ServiceClient, opts SubnetCreateOpts) (*Subnet, error) {
	createOpts := subnets.CreateOpts{
		NetworkID:  opts.NetworkID,
		Name:       opts.Name,
		CIDR:       opts.CIDR,
		IPVersion:  gophercloud.IPVersion(opts.IPVersion),
		EnableDHCP: &opts.EnableDHCP,
	}
	if opts.GatewayIP != "" {
		createOpts.GatewayIP = &opts.GatewayIP
	}
	if opts.SubnetPoolID != "" {
		createOpts.SubnetPoolID = opts.SubnetPoolID
	}
	if opts.PrefixLen > 0 {
		createOpts.Prefixlen = opts.PrefixLen
	}
	if opts.IPv6AddressMode != "" {
		createOpts.IPv6AddressMode = opts.IPv6AddressMode
	}
	if opts.IPv6RAMode != "" {
		createOpts.IPv6RAMode = opts.IPv6RAMode
	}
	r := subnets.Create(ctx, client, createOpts)
	s, err := r.Extract()
	if err != nil {
		return nil, fmt.Errorf("creating subnet: %w", err)
	}
	sub := &Subnet{
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
	return sub, nil
}

// DeleteSubnet deletes a subnet.
func DeleteSubnet(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := subnets.Delete(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("deleting subnet %s: %w", id, r.Err)
	}
	return nil
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
