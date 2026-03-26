package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/routers"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// Router is a simplified representation of a Neutron router.
type Router struct {
	ID                       string
	Name                     string
	Description              string
	Status                   string
	AdminStateUp             bool
	ExternalGatewayNetworkID string
	ExternalGatewayIP        string // first fixed IP if present
	Routes                   []Route
}

// Route is a static route on a router.
type Route struct {
	DestinationCIDR string
	NextHop         string
}

// RouterInterface represents a router's internal interface.
type RouterInterface struct {
	SubnetID  string
	PortID    string
	IPAddress string
}

// ListRouters fetches all routers.
func ListRouters(ctx context.Context, client *gophercloud.ServiceClient) ([]Router, error) {
	var result []Router
	err := routers.List(client, routers.ListOpts{}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := routers.ExtractRouters(page)
		if err != nil {
			return false, err
		}
		for _, r := range extracted {
			router := Router{
				ID:           r.ID,
				Name:         r.Name,
				Description:  r.Description,
				Status:       r.Status,
				AdminStateUp: r.AdminStateUp,
			}
			if r.GatewayInfo.NetworkID != "" {
				router.ExternalGatewayNetworkID = r.GatewayInfo.NetworkID
				if len(r.GatewayInfo.ExternalFixedIPs) > 0 {
					router.ExternalGatewayIP = r.GatewayInfo.ExternalFixedIPs[0].IPAddress
				}
			}
			for _, route := range r.Routes {
				router.Routes = append(router.Routes, Route{
					DestinationCIDR: route.DestinationCIDR,
					NextHop:         route.NextHop,
				})
			}
			result = append(result, router)
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing routers: %w", err)
	}
	return result, nil
}

// GetRouter fetches a single router.
func GetRouter(ctx context.Context, client *gophercloud.ServiceClient, id string) (*Router, error) {
	r, err := routers.Get(ctx, client, id).Extract()
	if err != nil {
		return nil, fmt.Errorf("getting router %s: %w", id, err)
	}
	router := &Router{
		ID:           r.ID,
		Name:         r.Name,
		Description:  r.Description,
		Status:       r.Status,
		AdminStateUp: r.AdminStateUp,
	}
	if r.GatewayInfo.NetworkID != "" {
		router.ExternalGatewayNetworkID = r.GatewayInfo.NetworkID
		if len(r.GatewayInfo.ExternalFixedIPs) > 0 {
			router.ExternalGatewayIP = r.GatewayInfo.ExternalFixedIPs[0].IPAddress
		}
	}
	for _, route := range r.Routes {
		router.Routes = append(router.Routes, Route{
			DestinationCIDR: route.DestinationCIDR,
			NextHop:         route.NextHop,
		})
	}
	return router, nil
}

// CreateRouter creates a new router.
func CreateRouter(ctx context.Context, client *gophercloud.ServiceClient, name, extNetworkID string, adminStateUp bool) (*Router, error) {
	opts := routers.CreateOpts{
		Name:         name,
		AdminStateUp: &adminStateUp,
	}
	if extNetworkID != "" {
		opts.GatewayInfo = &routers.GatewayInfo{
			NetworkID: extNetworkID,
		}
	}
	r, err := routers.Create(ctx, client, opts).Extract()
	if err != nil {
		return nil, fmt.Errorf("creating router: %w", err)
	}
	router := &Router{
		ID:           r.ID,
		Name:         r.Name,
		Status:       r.Status,
		AdminStateUp: r.AdminStateUp,
	}
	if r.GatewayInfo.NetworkID != "" {
		router.ExternalGatewayNetworkID = r.GatewayInfo.NetworkID
	}
	return router, nil
}

// DeleteRouter deletes a router.
func DeleteRouter(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := routers.Delete(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("deleting router %s: %w", id, r.Err)
	}
	return nil
}

// AddRouterInterface attaches a subnet to a router.
func AddRouterInterface(ctx context.Context, client *gophercloud.ServiceClient, routerID, subnetID string) error {
	_, err := routers.AddInterface(ctx, client, routerID, routers.AddInterfaceOpts{
		SubnetID: subnetID,
	}).Extract()
	if err != nil {
		return fmt.Errorf("adding interface to router %s: %w", routerID, err)
	}
	return nil
}

// RemoveRouterInterface detaches a subnet from a router.
func RemoveRouterInterface(ctx context.Context, client *gophercloud.ServiceClient, routerID, subnetID string) error {
	_, err := routers.RemoveInterface(ctx, client, routerID, routers.RemoveInterfaceOpts{
		SubnetID: subnetID,
	}).Extract()
	if err != nil {
		return fmt.Errorf("removing interface from router %s: %w", routerID, err)
	}
	return nil
}

// ListRouterInterfaces lists ports owned by a router (device_owner=network:router_interface).
func ListRouterInterfaces(ctx context.Context, client *gophercloud.ServiceClient, routerID string) ([]RouterInterface, error) {
	var result []RouterInterface
	err := ports.List(client, ports.ListOpts{
		DeviceID:    routerID,
		DeviceOwner: "network:router_interface",
	}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := ports.ExtractPorts(page)
		if err != nil {
			return false, err
		}
		for _, p := range extracted {
			for _, ip := range p.FixedIPs {
				result = append(result, RouterInterface{
					SubnetID:  ip.SubnetID,
					PortID:    p.ID,
					IPAddress: ip.IPAddress,
				})
			}
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing router interfaces for %s: %w", routerID, err)
	}
	return result, nil
}
