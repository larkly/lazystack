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
	Description   string
	Protocol      string
	ProtocolPort  int
	DefaultPoolID string
	ConnLimit     int
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
	AdminStateUp    bool
	OperatingStatus string
	Backup          bool
	MonitorAddress  string
	MonitorPort     int
	Tags            []string
}

// MemberCreateOpts contains editable fields for creating a pool member.
type MemberCreateOpts struct {
	Name           string
	Address        string
	ProtocolPort   int
	Weight         int
	AdminStateUp   bool
	Backup         bool
	MonitorAddress string
	MonitorPort    *int
	Tags           []string
}

// MemberUpdateOpts contains editable fields for updating a pool member.
type MemberUpdateOpts struct {
	Name              *string
	Weight            *int
	AdminStateUp      *bool
	Backup            *bool
	MonitorAddress    *string
	MonitorAddressSet bool
	MonitorPort       *int
	MonitorPortSet    bool
	Tags              *[]string
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

// CreateLoadBalancer creates a new load balancer on the given subnet.
func CreateLoadBalancer(ctx context.Context, client *gophercloud.ServiceClient, name, description, vipSubnetID string) (*LoadBalancer, error) {
	opts := loadbalancers.CreateOpts{
		Name:        name,
		Description: description,
		VipSubnetID: vipSubnetID,
	}
	lb, err := loadbalancers.Create(ctx, client, opts).Extract()
	if err != nil {
		return nil, fmt.Errorf("creating load balancer: %w", err)
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

// UpdateLoadBalancer updates a load balancer's name and/or description.
func UpdateLoadBalancer(ctx context.Context, client *gophercloud.ServiceClient, id string, name, description *string) error {
	opts := loadbalancers.UpdateOpts{
		Name:        name,
		Description: description,
	}
	_, err := loadbalancers.Update(ctx, client, id, opts).Extract()
	if err != nil {
		return fmt.Errorf("updating load balancer %s: %w", id, err)
	}
	return nil
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
				Description:   l.Description,
				Protocol:      l.Protocol,
				ProtocolPort:  l.ProtocolPort,
				DefaultPoolID: l.DefaultPoolID,
				ConnLimit:     l.ConnLimit,
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
		Description:   l.Description,
		Protocol:      l.Protocol,
		ProtocolPort:  l.ProtocolPort,
		DefaultPoolID: l.DefaultPoolID,
		ConnLimit:     l.ConnLimit,
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

// UpdateListener updates a listener's name.
func UpdateListener(ctx context.Context, client *gophercloud.ServiceClient, id string, name, description *string, connLimit *int) error {
	opts := listeners.UpdateOpts{
		Name:        name,
		Description: description,
		ConnLimit:   connLimit,
	}
	_, err := listeners.Update(ctx, client, id, opts).Extract()
	if err != nil {
		return fmt.Errorf("updating listener %s: %w", id, err)
	}
	return nil
}

// CreatePool creates a pool on a load balancer and, when requested, creates its
// health monitor as a follow-up operation.
func CreatePool(ctx context.Context, client *gophercloud.ServiceClient, lbID, name, protocol, lbMethod string, mon *monitors.CreateOpts) (*Pool, error) {
	opts := pools.CreateOpts{
		LoadbalancerID: lbID,
		Name:           name,
		Protocol:       pools.Protocol(protocol),
		LBMethod:       pools.LBMethod(lbMethod),
	}
	p, err := pools.Create(ctx, client, opts).Extract()
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}
	result := &Pool{
		ID:        p.ID,
		Name:      p.Name,
		Protocol:  p.Protocol,
		LBMethod:  p.LBMethod,
		MonitorID: p.MonitorID,
	}
	if mon == nil {
		return result, nil
	}

	monOpts := *mon
	monOpts.PoolID = p.ID

	createdMon, err := monitors.Create(ctx, client, monOpts).Extract()
	if err != nil {
		if deleteErr := DeletePool(ctx, client, p.ID); deleteErr != nil {
			return nil, fmt.Errorf("creating health monitor for pool %s: %w (cleanup failed: %v)", p.ID, err, deleteErr)
		}
		return nil, fmt.Errorf("creating health monitor for pool %s: %w", p.ID, err)
	}

	result.MonitorID = createdMon.ID
	return result, nil
}

// DeletePool deletes a pool.
func DeletePool(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := pools.Delete(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("deleting pool %s: %w", id, r.Err)
	}
	return nil
}

// UpdatePool updates a pool's name and/or LB method.
func UpdatePool(ctx context.Context, client *gophercloud.ServiceClient, id string, name *string, lbMethod string) error {
	opts := pools.UpdateOpts{
		Name: name,
	}
	if lbMethod != "" {
		opts.LBMethod = pools.LBMethod(lbMethod)
	}
	_, err := pools.Update(ctx, client, id, opts).Extract()
	if err != nil {
		return fmt.Errorf("updating pool %s: %w", id, err)
	}
	return nil
}

// CreateMember adds a member to a pool.
func CreateMember(ctx context.Context, client *gophercloud.ServiceClient, poolID string, opts MemberCreateOpts) (*Member, error) {
	createOpts := pools.CreateMemberOpts{
		Name:           opts.Name,
		Address:        opts.Address,
		ProtocolPort:   opts.ProtocolPort,
		Weight:         &opts.Weight,
		AdminStateUp:   &opts.AdminStateUp,
		Backup:         &opts.Backup,
		MonitorAddress: opts.MonitorAddress,
		MonitorPort:    opts.MonitorPort,
		Tags:           cloneStringSlice(opts.Tags),
	}
	m, err := pools.CreateMember(ctx, client, poolID, createOpts).Extract()
	if err != nil {
		return nil, fmt.Errorf("creating member: %w", err)
	}
	member := simplifyMember(m)
	return &member, nil
}

// DeleteMember removes a member from a pool.
func DeleteMember(ctx context.Context, client *gophercloud.ServiceClient, poolID, memberID string) error {
	r := pools.DeleteMember(ctx, client, poolID, memberID)
	if r.Err != nil {
		return fmt.Errorf("deleting member %s: %w", memberID, r.Err)
	}
	return nil
}

// UpdateMember updates an existing member.
func UpdateMember(ctx context.Context, client *gophercloud.ServiceClient, poolID, memberID string, opts MemberUpdateOpts) error {
	_, err := pools.UpdateMember(ctx, client, poolID, memberID, memberUpdateRequest(opts)).Extract()
	if err != nil {
		return fmt.Errorf("updating member %s: %w", memberID, err)
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
			result = append(result, simplifyMember(&m))
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing members for pool %s: %w", poolID, err)
	}
	return result, nil
}

type memberUpdateRequest MemberUpdateOpts

func (opts memberUpdateRequest) ToMemberUpdateMap() (map[string]any, error) {
	body := map[string]any{}
	if opts.Name != nil {
		body["name"] = *opts.Name
	}
	if opts.Weight != nil {
		body["weight"] = *opts.Weight
	}
	if opts.AdminStateUp != nil {
		body["admin_state_up"] = *opts.AdminStateUp
	}
	if opts.Backup != nil {
		body["backup"] = *opts.Backup
	}
	if opts.MonitorAddressSet {
		if opts.MonitorAddress == nil {
			body["monitor_address"] = nil
		} else {
			body["monitor_address"] = *opts.MonitorAddress
		}
	}
	if opts.MonitorPortSet {
		if opts.MonitorPort == nil {
			body["monitor_port"] = nil
		} else {
			body["monitor_port"] = *opts.MonitorPort
		}
	}
	if opts.Tags != nil {
		if len(*opts.Tags) == 0 {
			body["tags"] = []string{}
		} else {
			body["tags"] = cloneStringSlice(*opts.Tags)
		}
	}
	return map[string]any{"member": body}, nil
}

func simplifyMember(m *pools.Member) Member {
	return Member{
		ID:              m.ID,
		Name:            m.Name,
		Address:         m.Address,
		ProtocolPort:    m.ProtocolPort,
		Weight:          m.Weight,
		AdminStateUp:    m.AdminStateUp,
		OperatingStatus: m.OperatingStatus,
		Backup:          m.Backup,
		MonitorAddress:  m.MonitorAddress,
		MonitorPort:     m.MonitorPort,
		Tags:            cloneStringSlice(m.Tags),
	}
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
