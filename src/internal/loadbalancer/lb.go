package loadbalancer

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/listeners"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/loadbalancers"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/monitors"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/pools"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// LoadBalancer is a simplified load balancer.
type LoadBalancer struct {
	ID                 string
	Name               string
	Description        string
	VipAddress         string
	VipSubnetID        string
	ProvisioningStatus string
	OperatingStatus    string
	Provider           string
}

// Listener is a simplified listener.
type Listener struct {
	ID            string
	Name          string
	Protocol      string
	ProtocolPort  int
	DefaultPoolID string
}

// Pool is a simplified pool.
type Pool struct {
	ID        string
	Name      string
	Protocol  string
	LBMethod  string
	MonitorID string
}

// Member is a simplified pool member.
type Member struct {
	ID              string
	Name            string
	Address         string
	ProtocolPort    int
	Weight          int
	OperatingStatus string
}

// HealthMonitor is a simplified health monitor.
type HealthMonitor struct {
	ID                 string
	Name               string
	Type               string
	Delay              int
	Timeout            int
	MaxRetries         int
	MaxRetriesDown     int
	HTTPMethod         string
	URLPath            string
	ExpectedCodes      string
	AdminStateUp       bool
	OperatingStatus    string
	ProvisioningStatus string
}

// ListLoadBalancers fetches all load balancers.
func ListLoadBalancers(ctx context.Context, client *gophercloud.ServiceClient) ([]LoadBalancer, error) {
	var result []LoadBalancer
	err := loadbalancers.List(client, loadbalancers.ListOpts{}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := loadbalancers.ExtractLoadBalancers(page)
		if err != nil {
			return false, err
		}
		for _, lb := range extracted {
			result = append(result, LoadBalancer{
				ID:                 lb.ID,
				Name:               lb.Name,
				Description:        lb.Description,
				VipAddress:         lb.VipAddress,
				VipSubnetID:        lb.VipSubnetID,
				ProvisioningStatus: lb.ProvisioningStatus,
				OperatingStatus:    lb.OperatingStatus,
				Provider:           lb.Provider,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing load balancers: %w", err)
	}
	return result, nil
}

// GetLoadBalancer fetches a single load balancer by ID.
func GetLoadBalancer(ctx context.Context, client *gophercloud.ServiceClient, id string) (*LoadBalancer, error) {
	lb, err := loadbalancers.Get(ctx, client, id).Extract()
	if err != nil {
		return nil, fmt.Errorf("getting load balancer %s: %w", id, err)
	}
	return &LoadBalancer{
		ID:                 lb.ID,
		Name:               lb.Name,
		Description:        lb.Description,
		VipAddress:         lb.VipAddress,
		VipSubnetID:        lb.VipSubnetID,
		ProvisioningStatus: lb.ProvisioningStatus,
		OperatingStatus:    lb.OperatingStatus,
		Provider:           lb.Provider,
	}, nil
}

// DeleteLoadBalancer deletes a load balancer with cascade.
func DeleteLoadBalancer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := loadbalancers.Delete(ctx, client, id, loadbalancers.DeleteOpts{Cascade: true})
	if r.Err != nil {
		return fmt.Errorf("deleting load balancer %s: %w", id, r.Err)
	}
	return nil
}

// ListListeners fetches listeners for a load balancer.
func ListListeners(ctx context.Context, client *gophercloud.ServiceClient, lbID string) ([]Listener, error) {
	var result []Listener
	err := listeners.List(client, listeners.ListOpts{LoadbalancerID: lbID}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := listeners.ExtractListeners(page)
		if err != nil {
			return false, err
		}
		for _, l := range extracted {
			result = append(result, Listener{
				ID:            l.ID,
				Name:          l.Name,
				Protocol:      l.Protocol,
				ProtocolPort:  l.ProtocolPort,
				DefaultPoolID: l.DefaultPoolID,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing listeners for LB %s: %w", lbID, err)
	}
	return result, nil
}

// ListPools fetches pools for a load balancer.
func ListPools(ctx context.Context, client *gophercloud.ServiceClient, lbID string) ([]Pool, error) {
	var result []Pool
	err := pools.List(client, pools.ListOpts{LoadbalancerID: lbID}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := pools.ExtractPools(page)
		if err != nil {
			return false, err
		}
		for _, p := range extracted {
			result = append(result, Pool{
				ID:        p.ID,
				Name:      p.Name,
				Protocol:  p.Protocol,
				LBMethod:  p.LBMethod,
				MonitorID: p.MonitorID,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing pools for LB %s: %w", lbID, err)
	}
	return result, nil
}

// GetHealthMonitor fetches a single health monitor by ID.
func GetHealthMonitor(ctx context.Context, client *gophercloud.ServiceClient, id string) (*HealthMonitor, error) {
	mon, err := monitors.Get(ctx, client, id).Extract()
	if err != nil {
		return nil, fmt.Errorf("getting health monitor %s: %w", id, err)
	}
	return &HealthMonitor{
		ID:                 mon.ID,
		Name:               mon.Name,
		Type:               mon.Type,
		Delay:              mon.Delay,
		Timeout:            mon.Timeout,
		MaxRetries:         mon.MaxRetries,
		MaxRetriesDown:     mon.MaxRetriesDown,
		HTTPMethod:         mon.HTTPMethod,
		URLPath:            mon.URLPath,
		ExpectedCodes:      mon.ExpectedCodes,
		AdminStateUp:       mon.AdminStateUp,
		OperatingStatus:    mon.OperatingStatus,
		ProvisioningStatus: mon.ProvisioningStatus,
	}, nil
}

// CreateListener creates a listener on a load balancer.
func CreateListener(ctx context.Context, client *gophercloud.ServiceClient, lbID, name, protocol string, port int) (*Listener, error) {
	opts := listeners.CreateOpts{
		LoadbalancerID: lbID,
		Name:           name,
		Protocol:       listeners.Protocol(protocol),
		ProtocolPort:   port,
	}
	l, err := listeners.Create(ctx, client, opts).Extract()
	if err != nil {
		return nil, fmt.Errorf("creating listener: %w", err)
	}
	return &Listener{
		ID:            l.ID,
		Name:          l.Name,
		Protocol:      l.Protocol,
		ProtocolPort:  l.ProtocolPort,
		DefaultPoolID: l.DefaultPoolID,
	}, nil
}

// DeleteListener deletes a listener.
func DeleteListener(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := listeners.Delete(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("deleting listener %s: %w", id, r.Err)
	}
	return nil
}

// CreatePool creates a pool on a load balancer, optionally with a health monitor.
func CreatePool(ctx context.Context, client *gophercloud.ServiceClient, lbID, name, protocol, lbMethod string, mon *monitors.CreateOpts) (*Pool, error) {
	opts := pools.CreateOpts{
		LoadbalancerID: lbID,
		Name:           name,
		Protocol:       pools.Protocol(protocol),
		LBMethod:       pools.LBMethod(lbMethod),
	}
	if mon != nil {
		opts.Monitor = mon
	}
	p, err := pools.Create(ctx, client, opts).Extract()
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}
	return &Pool{
		ID:        p.ID,
		Name:      p.Name,
		Protocol:  p.Protocol,
		LBMethod:  p.LBMethod,
		MonitorID: p.MonitorID,
	}, nil
}

// DeletePool deletes a pool.
func DeletePool(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := pools.Delete(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("deleting pool %s: %w", id, r.Err)
	}
	return nil
}

// CreateMember adds a member to a pool.
func CreateMember(ctx context.Context, client *gophercloud.ServiceClient, poolID, name, address string, port, weight int) (*Member, error) {
	opts := pools.CreateMemberOpts{
		Name:         name,
		Address:      address,
		ProtocolPort: port,
		Weight:       &weight,
	}
	m, err := pools.CreateMember(ctx, client, poolID, opts).Extract()
	if err != nil {
		return nil, fmt.Errorf("creating member: %w", err)
	}
	return &Member{
		ID:              m.ID,
		Name:            m.Name,
		Address:         m.Address,
		ProtocolPort:    m.ProtocolPort,
		Weight:          m.Weight,
		OperatingStatus: m.OperatingStatus,
	}, nil
}

// DeleteMember removes a member from a pool.
func DeleteMember(ctx context.Context, client *gophercloud.ServiceClient, poolID, memberID string) error {
	r := pools.DeleteMember(ctx, client, poolID, memberID)
	if r.Err != nil {
		return fmt.Errorf("deleting member %s: %w", memberID, r.Err)
	}
	return nil
}

// CreateHealthMonitor creates a health monitor for a pool.
func CreateHealthMonitor(ctx context.Context, client *gophercloud.ServiceClient, poolID, monType string, delay, timeout, maxRetries int, urlPath, expectedCodes, httpMethod string) (*HealthMonitor, error) {
	opts := monitors.CreateOpts{
		PoolID:     poolID,
		Type:       monType,
		Delay:      delay,
		Timeout:    timeout,
		MaxRetries: maxRetries,
	}
	if urlPath != "" {
		opts.URLPath = urlPath
	}
	if expectedCodes != "" {
		opts.ExpectedCodes = expectedCodes
	}
	if httpMethod != "" {
		opts.HTTPMethod = httpMethod
	}
	mon, err := monitors.Create(ctx, client, opts).Extract()
	if err != nil {
		return nil, fmt.Errorf("creating health monitor: %w", err)
	}
	return &HealthMonitor{
		ID:                 mon.ID,
		Name:               mon.Name,
		Type:               mon.Type,
		Delay:              mon.Delay,
		Timeout:            mon.Timeout,
		MaxRetries:         mon.MaxRetries,
		MaxRetriesDown:     mon.MaxRetriesDown,
		HTTPMethod:         mon.HTTPMethod,
		URLPath:            mon.URLPath,
		ExpectedCodes:      mon.ExpectedCodes,
		AdminStateUp:       mon.AdminStateUp,
		OperatingStatus:    mon.OperatingStatus,
		ProvisioningStatus: mon.ProvisioningStatus,
	}, nil
}

// DeleteHealthMonitor deletes a health monitor.
func DeleteHealthMonitor(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := monitors.Delete(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("deleting health monitor %s: %w", id, r.Err)
	}
	return nil
}

// ListMembers fetches members for a pool.
func ListMembers(ctx context.Context, client *gophercloud.ServiceClient, poolID string) ([]Member, error) {
	var result []Member
	err := pools.ListMembers(client, poolID, pools.ListMembersOpts{}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := pools.ExtractMembers(page)
		if err != nil {
			return false, err
		}
		for _, m := range extracted {
			result = append(result, Member{
				ID:              m.ID,
				Name:            m.Name,
				Address:         m.Address,
				ProtocolPort:    m.ProtocolPort,
				Weight:          m.Weight,
				OperatingStatus: m.OperatingStatus,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing members for pool %s: %w", poolID, err)
	}
	return result, nil
}
