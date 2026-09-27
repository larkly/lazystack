package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/routers"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/larkly/lazystack/internal/shared"
)

// Router is a simplified representation of a Neutron router.
type Router struct {
	ID                       string
	Name                     string
	Description              string
	Status                   string
	AdminStateUp             bool
	ExternalGatewayNetworkID string
	ExternalGatewayIPv4      string
	ExternalGatewayIPv6      string
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
	shared.Debugf("[network] listing routers")
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
				for _, fip := range r.GatewayInfo.ExternalFixedIPs {
					classifyGatewayIP(&router, fip.IPAddress)
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
		shared.Debugf("[network] list routers: %v", err)
		return nil, fmt.Errorf("listing routers: %w", err)
	}
	shared.Debugf("[network] listed %d routers", len(result))
	return result, nil
}

// GetRouter fetches a single router.
func GetRouter(ctx context.Context, client *gophercloud.ServiceClient, id string) (*Router, error) {
	shared.Debugf("[network] getting router %s", id)
	r, err := routers.Get(ctx, client, id).Extract()
	if err != nil {
		shared.Debugf("[network] get router %s: %v", id, err)
		return nil, fmt.Errorf("getting router %s: %w", id, err)
	}
	shared.Debugf("[network] got router %s (name: %q)", id, r.Name)
	router := &Router{
		ID:           r.ID,
		Name:         r.Name,
		Description:  r.Description,
		Status:       r.Status,
		AdminStateUp: r.AdminStateUp,
	}
	if r.GatewayInfo.NetworkID != "" {
		router.ExternalGatewayNetworkID = r.GatewayInfo.NetworkID
		for _, fip := range r.GatewayInfo.ExternalFixedIPs {
			classifyGatewayIP(router, fip.IPAddress)
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
	shared.Debugf("[network] creating router %q (external network: %s)", name, extNetworkID)
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
		shared.Debugf("[network] create router %q: %v", name, err)
		return nil, fmt.Errorf("creating router: %w", err)
	}
	shared.Debugf("[network] created router %q (ID: %s)", r.Name, r.ID)
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
	shared.Debugf("[network] deleting router %s", id)
	r := routers.Delete(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[network] delete router %s: %v", id, r.Err)
		return fmt.Errorf("deleting router %s: %w", id, r.Err)
	}
	shared.Debugf("[network] deleted router %s", id)
	return nil
}

// AddRouterInterface attaches a subnet to a router.
func AddRouterInterface(ctx context.Context, client *gophercloud.ServiceClient, routerID, subnetID string) error {
	shared.Debugf("[network] adding interface to router %s (subnet: %s)", routerID, subnetID)
	_, err := routers.AddInterface(ctx, client, routerID, routers.AddInterfaceOpts{
		SubnetID: subnetID,
	}).Extract()
	if err != nil {
		shared.Debugf("[network] add interface to router %s (subnet: %s): %v", routerID, subnetID, err)
		return fmt.Errorf("adding interface to router %s: %w", routerID, err)
	}
	shared.Debugf("[network] added interface to router %s (subnet: %s)", routerID, subnetID)
	return nil
}

// AddRouterInterfaceByPort attaches a pre-created port to a router.
func AddRouterInterfaceByPort(ctx context.Context, client *gophercloud.ServiceClient, routerID, portID string) error {
	shared.Debugf("[network] adding port interface to router %s (port: %s)", routerID, portID)
	_, err := routers.AddInterface(ctx, client, routerID, routers.AddInterfaceOpts{
		PortID: portID,
	}).Extract()
	if err != nil {
		shared.Debugf("[network] add port interface to router %s (port: %s): %v", routerID, portID, err)
		return fmt.Errorf("adding port interface to router %s: %w", routerID, err)
	}
	shared.Debugf("[network] added port interface to router %s (port: %s)", routerID, portID)
	return nil
}

// RemoveRouterInterface detaches a subnet from a router.
func RemoveRouterInterface(ctx context.Context, client *gophercloud.ServiceClient, routerID, subnetID string) error {
	shared.Debugf("[network] removing interface from router %s (subnet: %s)", routerID, subnetID)
	_, err := routers.RemoveInterface(ctx, client, routerID, routers.RemoveInterfaceOpts{
		SubnetID: subnetID,
	}).Extract()
	if err != nil {
		shared.Debugf("[network] remove interface from router %s (subnet: %s): %v", routerID, subnetID, err)
		return fmt.Errorf("removing interface from router %s: %w", routerID, err)
	}
	shared.Debugf("[network] removed interface from router %s (subnet: %s)", routerID, subnetID)
	return nil
}

// routerInterfaceOwners are the device_owner values Neutron uses for a
// router's subnet interfaces: legacy, HA (replicated) and distributed (DVR)
// routers. Gateway ports, HA-network ports (network:router_ha_interface) and
// DVR SNAT ports are auxiliary and deliberately not listed.
var routerInterfaceOwners = map[string]bool{
	"network:router_interface":               true,
	"network:ha_router_replicated_interface": true,
	"network:router_interface_distributed":   true,
}

// IsRouterInterfaceOwner reports whether a port device_owner denotes a
// router subnet interface (legacy, HA or DVR).
func IsRouterInterfaceOwner(owner string) bool {
	return routerInterfaceOwners[owner]
}

// isRouterInterfacePort reports whether p, listed with a device_id filter
// for routerID, is one of that router's subnet interfaces.
func isRouterInterfacePort(p ports.Port, routerID string) bool {
	if p.DeviceID != "" && p.DeviceID != routerID {
		return false
	}
	return IsRouterInterfaceOwner(p.DeviceOwner)
}

// ListRouterInterfaces lists a router's subnet interface ports, including
// HA and DVR interfaces.
func ListRouterInterfaces(ctx context.Context, client *gophercloud.ServiceClient, routerID string) ([]RouterInterface, error) {
	shared.Debugf("[network] listing router interfaces for router %s", routerID)
	var result []RouterInterface
	err := ports.List(client, ports.ListOpts{
		DeviceID: routerID,
	}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := ports.ExtractPorts(page)
		if err != nil {
			return false, err
		}
		for _, p := range extracted {
			if !isRouterInterfacePort(p, routerID) {
				continue
			}
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
		shared.Debugf("[network] list router interfaces for %s: %v", routerID, err)
		return nil, fmt.Errorf("listing router interfaces for %s: %w", routerID, err)
	}
	shared.Debugf("[network] listed %d router interfaces for router %s", len(result), routerID)
	return result, nil
}

func classifyGatewayIP(r *Router, ip string) {
	if strings.Contains(ip, ":") {
		if r.ExternalGatewayIPv6 == "" {
			r.ExternalGatewayIPv6 = ip
		}
	} else {
		if r.ExternalGatewayIPv4 == "" {
			r.ExternalGatewayIPv4 = ip
		}
	}
}
