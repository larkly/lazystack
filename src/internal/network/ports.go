package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// Port is a simplified representation of a Neutron port.
type Port struct {
	ID             string
	Name           string
	Status         string
	MACAddress     string
	FixedIPs       []FixedIP
	DeviceOwner    string
	DeviceID       string
	NetworkID      string
	SecurityGroups []string
	AdminStateUp   bool
}

// FixedIP is an IP address assigned to a port.
type FixedIP struct {
	SubnetID  string
	IPAddress string
}

// ListPortsByDevice fetches all ports attached to a given device (e.g. server).
func ListPortsByDevice(ctx context.Context, client *gophercloud.ServiceClient, deviceID string) ([]Port, error) {
	var result []Port
	err := ports.List(client, ports.ListOpts{DeviceID: deviceID}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := ports.ExtractPorts(page)
		if err != nil {
			return false, err
		}
		for _, p := range extracted {
			port := Port{
				ID:             p.ID,
				Name:           p.Name,
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
			result = append(result, port)
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
		extracted, err := ports.ExtractPorts(page)
		if err != nil {
			return false, err
		}
		for _, p := range extracted {
			port := Port{
				ID:             p.ID,
				Name:           p.Name,
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
			result = append(result, port)
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
	port := &Port{
		ID:           p.ID,
		Name:         p.Name,
		Status:       p.Status,
		MACAddress:   p.MACAddress,
		DeviceOwner:  p.DeviceOwner,
		DeviceID:     p.DeviceID,
		NetworkID:    p.NetworkID,
		AdminStateUp: p.AdminStateUp,
	}
	for _, ip := range p.FixedIPs {
		port.FixedIPs = append(port.FixedIPs, FixedIP{
			SubnetID:  ip.SubnetID,
			IPAddress: ip.IPAddress,
		})
	}
	return port, nil
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
		extracted, err := ports.ExtractPorts(page)
		if err != nil {
			return false, err
		}
		for _, p := range extracted {
			port := Port{
				ID:             p.ID,
				Name:           p.Name,
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
			result = append(result, port)
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing ports for network %s: %w", networkID, err)
	}
	return result, nil
}
