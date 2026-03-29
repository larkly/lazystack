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
	ID          string
	Name        string
	Status      string
	MACAddress  string
	FixedIPs    []FixedIP
	DeviceOwner string
	DeviceID    string
	NetworkID   string
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
				ID:          p.ID,
				Name:        p.Name,
				Status:      p.Status,
				MACAddress:  p.MACAddress,
				DeviceOwner: p.DeviceOwner,
				DeviceID:    p.DeviceID,
				NetworkID:   p.NetworkID,
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
				ID:          p.ID,
				Name:        p.Name,
				Status:      p.Status,
				MACAddress:  p.MACAddress,
				DeviceOwner: p.DeviceOwner,
				DeviceID:    p.DeviceID,
				NetworkID:   p.NetworkID,
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
