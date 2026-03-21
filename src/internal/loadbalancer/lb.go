package loadbalancer

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/listeners"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/loadbalancers"
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
	ID       string
	Name     string
	Protocol string
	LBMethod string
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
				ID:       p.ID,
				Name:     p.Name,
				Protocol: p.Protocol,
				LBMethod: p.LBMethod,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing pools for LB %s: %w", lbID, err)
	}
	return result, nil
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
