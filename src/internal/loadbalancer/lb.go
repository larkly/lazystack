package loadbalancer

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/shared"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/listeners"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/loadbalancers"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/monitors"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/pools"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// requireLBClient guards against a nil Octavia client on clouds lacking the
// load balancer service; gophercloud methods panic on nil *ServiceClient
// receivers.
func requireLBClient(client *gophercloud.ServiceClient) error {
	if client == nil {
		return fmt.Errorf("load balancer (Octavia) service is not available in this cloud")
	}
	return nil
}

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
	AdminStateUp       bool
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
	AdminStateUp  bool
}

// Pool is a simplified pool.
type Pool struct {
	ID           string
	Name         string
	Protocol     string
	LBMethod     string
	MonitorID    string
	AdminStateUp bool
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
	if err := requireLBClient(client); err != nil {
		return nil, err
	}
	shared.Debugf("[lb] ListLoadBalancers: starting")
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
				AdminStateUp:       lb.AdminStateUp,
			})
		}
		return true, nil
	})
	if err != nil {
		shared.Debugf("[lb] ListLoadBalancers: error: %v", err)
		return nil, fmt.Errorf("listing load balancers: %w", err)
	}
	shared.Debugf("[lb] ListLoadBalancers: success, count=%d", len(result))
	return result, nil
}

// GetLoadBalancer fetches a single load balancer by ID.
func GetLoadBalancer(ctx context.Context, client *gophercloud.ServiceClient, id string) (*LoadBalancer, error) {
	if err := requireLBClient(client); err != nil {
		return nil, err
	}
	shared.Debugf("[lb] GetLoadBalancer: starting, id=%s", id)
	lb, err := loadbalancers.Get(ctx, client, id).Extract()
	if err != nil {
		shared.Debugf("[lb] GetLoadBalancer: error: %v", err)
		return nil, fmt.Errorf("getting load balancer %s: %w", id, err)
	}
	shared.Debugf("[lb] GetLoadBalancer: success, id=%s name=%s", id, lb.Name)
	return &LoadBalancer{
		ID:                 lb.ID,
		Name:               lb.Name,
		Description:        lb.Description,
		VipAddress:         lb.VipAddress,
		VipSubnetID:        lb.VipSubnetID,
		ProvisioningStatus: lb.ProvisioningStatus,
		OperatingStatus:    lb.OperatingStatus,
		Provider:           lb.Provider,
		AdminStateUp:       lb.AdminStateUp,
	}, nil
}

// CreateLoadBalancer creates a new load balancer on the given subnet.
func CreateLoadBalancer(ctx context.Context, client *gophercloud.ServiceClient, name, description, vipSubnetID string) (*LoadBalancer, error) {
	if err := requireLBClient(client); err != nil {
		return nil, err
	}
	shared.Debugf("[lb] CreateLoadBalancer: starting, name=%s subnetID=%s", name, vipSubnetID)
	opts := loadbalancers.CreateOpts{
		Name:        name,
		Description: description,
		VipSubnetID: vipSubnetID,
	}
	lb, err := loadbalancers.Create(ctx, client, opts).Extract()
	if err != nil {
		shared.Debugf("[lb] CreateLoadBalancer: error: %v", err)
		return nil, fmt.Errorf("creating load balancer: %w", err)
	}
	shared.Debugf("[lb] CreateLoadBalancer: success, id=%s name=%s", lb.ID, lb.Name)
	return &LoadBalancer{
		ID:                 lb.ID,
		Name:               lb.Name,
		Description:        lb.Description,
		VipAddress:         lb.VipAddress,
		VipSubnetID:        lb.VipSubnetID,
		ProvisioningStatus: lb.ProvisioningStatus,
		OperatingStatus:    lb.OperatingStatus,
		Provider:           lb.Provider,
		AdminStateUp:       lb.AdminStateUp,
	}, nil
}

// UpdateLoadBalancer updates a load balancer's name and/or description.
func UpdateLoadBalancer(ctx context.Context, client *gophercloud.ServiceClient, id string, name, description *string, adminStateUp *bool) error {
	if err := requireLBClient(client); err != nil {
		return err
	}
	shared.Debugf("[lb] UpdateLoadBalancer: starting, id=%s", id)
	opts := loadbalancers.UpdateOpts{
		Name:         name,
		Description:  description,
		AdminStateUp: adminStateUp,
	}
	_, err := loadbalancers.Update(ctx, client, id, opts).Extract()
	if err != nil {
		shared.Debugf("[lb] UpdateLoadBalancer: error: %v", err)
		return fmt.Errorf("updating load balancer %s: %w", id, err)
	}
	shared.Debugf("[lb] UpdateLoadBalancer: success, id=%s", id)
	return nil
}

// DeleteLoadBalancer deletes a load balancer with cascade.
func DeleteLoadBalancer(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	if err := requireLBClient(client); err != nil {
		return err
	}
	shared.Debugf("[lb] DeleteLoadBalancer: starting, id=%s", id)
	r := loadbalancers.Delete(ctx, client, id, loadbalancers.DeleteOpts{Cascade: true})
	if r.Err != nil {
		shared.Debugf("[lb] DeleteLoadBalancer: error: %v", r.Err)
		return fmt.Errorf("deleting load balancer %s: %w", id, r.Err)
	}
	shared.Debugf("[lb] DeleteLoadBalancer: success, id=%s", id)
	return nil
}

// ListListeners fetches listeners for a load balancer.
func ListListeners(ctx context.Context, client *gophercloud.ServiceClient, lbID string) ([]Listener, error) {
	if err := requireLBClient(client); err != nil {
		return nil, err
	}
	shared.Debugf("[lb] ListListeners: starting, lbID=%s", lbID)
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
				AdminStateUp:  l.AdminStateUp,
			})
		}
		return true, nil
	})
	if err != nil {
		shared.Debugf("[lb] ListListeners: error: %v", err)
		return nil, fmt.Errorf("listing listeners for LB %s: %w", lbID, err)
	}
	shared.Debugf("[lb] ListListeners: success, lbID=%s count=%d", lbID, len(result))
	return result, nil
}

// ListPools fetches pools for a load balancer.
func ListPools(ctx context.Context, client *gophercloud.ServiceClient, lbID string) ([]Pool, error) {
	if err := requireLBClient(client); err != nil {
		return nil, err
	}
	shared.Debugf("[lb] ListPools: starting, lbID=%s", lbID)
	var result []Pool
	err := pools.List(client, pools.ListOpts{LoadbalancerID: lbID}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := pools.ExtractPools(page)
		if err != nil {
			return false, err
		}
		for _, p := range extracted {
			result = append(result, Pool{
				ID:           p.ID,
				Name:         p.Name,
				Protocol:     p.Protocol,
				LBMethod:     p.LBMethod,
				MonitorID:    p.MonitorID,
				AdminStateUp: p.AdminStateUp,
			})
		}
		return true, nil
	})
	if err != nil {
		shared.Debugf("[lb] ListPools: error: %v", err)
		return nil, fmt.Errorf("listing pools for LB %s: %w", lbID, err)
	}
	shared.Debugf("[lb] ListPools: success, lbID=%s count=%d", lbID, len(result))
	return result, nil
}

// GetHealthMonitor fetches a single health monitor by ID.
func GetHealthMonitor(ctx context.Context, client *gophercloud.ServiceClient, id string) (*HealthMonitor, error) {
	if err := requireLBClient(client); err != nil {
		return nil, err
	}
	shared.Debugf("[lb] GetHealthMonitor: starting, id=%s", id)
	mon, err := monitors.Get(ctx, client, id).Extract()
	if err != nil {
		shared.Debugf("[lb] GetHealthMonitor: error: %v", err)
		return nil, fmt.Errorf("getting health monitor %s: %w", id, err)
	}
	shared.Debugf("[lb] GetHealthMonitor: success, id=%s type=%s", mon.ID, mon.Type)
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
	if err := requireLBClient(client); err != nil {
		return nil, err
	}
	shared.Debugf("[lb] CreateListener: starting, lbID=%s name=%s protocol=%s port=%d", lbID, name, protocol, port)
	opts := listeners.CreateOpts{
		LoadbalancerID: lbID,
		Name:           name,
		Protocol:       listeners.Protocol(protocol),
		ProtocolPort:   port,
	}
	l, err := listeners.Create(ctx, client, opts).Extract()
	if err != nil {
		shared.Debugf("[lb] CreateListener: error: %v", err)
		return nil, fmt.Errorf("creating listener: %w", err)
	}
	shared.Debugf("[lb] CreateListener: success, id=%s name=%s", l.ID, l.Name)
	return &Listener{
		ID:            l.ID,
		Name:          l.Name,
		Description:   l.Description,
		Protocol:      l.Protocol,
		ProtocolPort:  l.ProtocolPort,
		DefaultPoolID: l.DefaultPoolID,
		ConnLimit:     l.ConnLimit,
		AdminStateUp:  l.AdminStateUp,
	}, nil
}

// DeleteListener deletes a listener.
func DeleteListener(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	if err := requireLBClient(client); err != nil {
		return err
	}
	shared.Debugf("[lb] DeleteListener: starting, id=%s", id)
	r := listeners.Delete(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[lb] DeleteListener: error: %v", r.Err)
		return fmt.Errorf("deleting listener %s: %w", id, r.Err)
	}
	shared.Debugf("[lb] DeleteListener: success, id=%s", id)
	return nil
}

// UpdateListener updates a listener's name.
func UpdateListener(ctx context.Context, client *gophercloud.ServiceClient, id string, name, description *string, connLimit *int, adminStateUp *bool) error {
	if err := requireLBClient(client); err != nil {
		return err
	}
	shared.Debugf("[lb] UpdateListener: starting, id=%s", id)
	opts := listeners.UpdateOpts{
		Name:         name,
		Description:  description,
		ConnLimit:    connLimit,
		AdminStateUp: adminStateUp,
	}
	_, err := listeners.Update(ctx, client, id, opts).Extract()
	if err != nil {
		shared.Debugf("[lb] UpdateListener: error: %v", err)
		return fmt.Errorf("updating listener %s: %w", id, err)
	}
	shared.Debugf("[lb] UpdateListener: success, id=%s", id)
	return nil
}

// CreatePool creates a pool on a load balancer and, when requested, creates its
// health monitor as a follow-up operation. Because pools are immutable while
// PENDING_CREATE, the pool is polled until ACTIVE before the monitor is created.
func CreatePool(ctx context.Context, client *gophercloud.ServiceClient, lbID, name, protocol, lbMethod string, mon *monitors.CreateOpts) (*Pool, error) {
	if err := requireLBClient(client); err != nil {
		return nil, err
	}
	shared.Debugf("[lb] CreatePool: starting, lbID=%s name=%s protocol=%s lbMethod=%s", lbID, name, protocol, lbMethod)
	opts := pools.CreateOpts{
		LoadbalancerID: lbID,
		Name:           name,
		Protocol:       pools.Protocol(protocol),
		LBMethod:       pools.LBMethod(lbMethod),
	}
	p, err := pools.Create(ctx, client, opts).Extract()
	if err != nil {
		shared.Debugf("[lb] CreatePool: error: %v", err)
		return nil, fmt.Errorf("creating pool: %w", err)
	}
	shared.Debugf("[lb] CreatePool: pool created, id=%s name=%s", p.ID, p.Name)
	result := &Pool{
		ID:           p.ID,
		Name:         p.Name,
		Protocol:     p.Protocol,
		LBMethod:     p.LBMethod,
		MonitorID:    p.MonitorID,
		AdminStateUp: p.AdminStateUp,
	}
	if mon == nil {
		shared.Debugf("[lb] CreatePool: success (no monitor), id=%s", p.ID)
		return result, nil
	}

	if err := waitForPoolActive(ctx, client, p.ID, 60*time.Second); err != nil {
		shared.Debugf("[lb] CreatePool: waiting for pool %s to become ACTIVE failed: %v, cleaning up", p.ID, err)
		cleanupPool(ctx, client, p.ID)
		return nil, fmt.Errorf("waiting for pool %s to become ACTIVE before creating health monitor: %w", p.ID, err)
	}

	shared.Debugf("[lb] CreatePool: creating health monitor for pool %s", p.ID)
	monOpts := *mon
	monOpts.PoolID = p.ID

	createdMon, err := monitors.Create(ctx, client, monOpts).Extract()
	if err != nil {
		shared.Debugf("[lb] CreatePool: health monitor creation failed: %v, cleaning up pool %s", err, p.ID)
		if deleteErr := DeletePool(ctx, client, p.ID); deleteErr != nil {
			if gophercloud.ResponseCodeIs(deleteErr, http.StatusConflict) || gophercloud.ResponseCodeIs(deleteErr, http.StatusNotFound) {
				shared.Debugf("[lb] CreatePool: cleanup delete of pool %s returned tolerable status (already deleting or gone), not masking original error: %v", p.ID, deleteErr)
			} else {
				return nil, fmt.Errorf("creating health monitor for pool %s: %w (cleanup failed: %v)", p.ID, err, deleteErr)
			}
		}
		return nil, fmt.Errorf("creating health monitor for pool %s: %w", p.ID, err)
	}

	result.MonitorID = createdMon.ID
	shared.Debugf("[lb] CreatePool: success with monitor, poolID=%s monitorID=%s", p.ID, createdMon.ID)
	return result, nil
}

// cleanupPool deletes a pool, tolerating 409/404 responses (the pool may be
// mid-delete or already gone).
func cleanupPool(ctx context.Context, client *gophercloud.ServiceClient, poolID string) {
	if err := DeletePool(ctx, client, poolID); err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusConflict) || gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			shared.Debugf("[lb] cleanupPool: delete of pool %s returned tolerable status, ignoring: %v", poolID, err)
			return
		}
		shared.Debugf("[lb] cleanupPool: delete of pool %s failed: %v", poolID, err)
	}
}

// waitForPoolActive polls the pool until its provisioning status is ACTIVE,
// the context is cancelled, or the timeout elapses. On deadline it returns an
// error so callers can proceed to their error/cleanup path.
func waitForPoolActive(ctx context.Context, client *gophercloud.ServiceClient, poolID string, timeout time.Duration) error {
	shared.Debugf("[lb] waitForPoolActive: starting, poolID=%s timeout=%s", poolID, timeout)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p, err := pools.Get(ctx, client, poolID).Extract()
		if err != nil {
			switch {
			case gophercloud.ResponseCodeIs(err, http.StatusNotFound):
				shared.Debugf("[lb] waitForPoolActive: pool not found, poolID=%s", poolID)
				return fmt.Errorf("pool %s not found", poolID)
			case gophercloud.ResponseCodeIs(err, http.StatusUnauthorized) || gophercloud.ResponseCodeIs(err, http.StatusForbidden):
				shared.Debugf("[lb] waitForPoolActive: polling error: %v", err)
				return fmt.Errorf("polling pool %s: %w", poolID, err)
			default:
				shared.Debugf("[lb] waitForPoolActive: transient polling error, retrying: %v", err)
			}
		} else {
			if p.ProvisioningStatus == "ACTIVE" {
				shared.Debugf("[lb] waitForPoolActive: success, poolID=%s is ACTIVE", poolID)
				return nil
			}
			if strings.HasPrefix(p.ProvisioningStatus, "ERROR") {
				shared.Debugf("[lb] waitForPoolActive: error status, poolID=%s status=%s", poolID, p.ProvisioningStatus)
				return fmt.Errorf("pool %s entered %s", poolID, p.ProvisioningStatus)
			}
		}
		select {
		case <-ctx.Done():
			shared.Debugf("[lb] waitForPoolActive: context cancelled, poolID=%s", poolID)
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	shared.Debugf("[lb] waitForPoolActive: timed out, poolID=%s", poolID)
	return fmt.Errorf("timed out waiting for pool %s to become ACTIVE", poolID)
}

// DeletePool deletes a pool.
func DeletePool(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	if err := requireLBClient(client); err != nil {
		return err
	}
	shared.Debugf("[lb] DeletePool: starting, id=%s", id)
	r := pools.Delete(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[lb] DeletePool: error: %v", r.Err)
		return fmt.Errorf("deleting pool %s: %w", id, r.Err)
	}
	shared.Debugf("[lb] DeletePool: success, id=%s", id)
	return nil
}

// UpdatePool updates a pool's name and/or LB method.
func UpdatePool(ctx context.Context, client *gophercloud.ServiceClient, id string, name *string, lbMethod string, adminStateUp *bool) error {
	if err := requireLBClient(client); err != nil {
		return err
	}
	shared.Debugf("[lb] UpdatePool: starting, id=%s", id)
	opts := pools.UpdateOpts{
		Name:         name,
		AdminStateUp: adminStateUp,
	}
	if lbMethod != "" {
		opts.LBMethod = pools.LBMethod(lbMethod)
	}
	_, err := pools.Update(ctx, client, id, opts).Extract()
	if err != nil {
		shared.Debugf("[lb] UpdatePool: error: %v", err)
		return fmt.Errorf("updating pool %s: %w", id, err)
	}
	shared.Debugf("[lb] UpdatePool: success, id=%s", id)
	return nil
}

// CreateMember adds a member to a pool.
func CreateMember(ctx context.Context, client *gophercloud.ServiceClient, poolID string, opts MemberCreateOpts) (*Member, error) {
	if err := requireLBClient(client); err != nil {
		return nil, err
	}
	shared.Debugf("[lb] CreateMember: starting, poolID=%s address=%s port=%d", poolID, opts.Address, opts.ProtocolPort)
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
		shared.Debugf("[lb] CreateMember: error: %v", err)
		return nil, fmt.Errorf("creating member: %w", err)
	}
	member := simplifyMember(m)
	shared.Debugf("[lb] CreateMember: success, poolID=%s memberID=%s", poolID, member.ID)
	return &member, nil
}

// DeleteMember removes a member from a pool.
func DeleteMember(ctx context.Context, client *gophercloud.ServiceClient, poolID, memberID string) error {
	if err := requireLBClient(client); err != nil {
		return err
	}
	shared.Debugf("[lb] DeleteMember: starting, poolID=%s memberID=%s", poolID, memberID)
	r := pools.DeleteMember(ctx, client, poolID, memberID)
	if r.Err != nil {
		shared.Debugf("[lb] DeleteMember: error: %v", r.Err)
		return fmt.Errorf("deleting member %s: %w", memberID, r.Err)
	}
	shared.Debugf("[lb] DeleteMember: success, poolID=%s memberID=%s", poolID, memberID)
	return nil
}

// UpdateMember updates an existing member.
func UpdateMember(ctx context.Context, client *gophercloud.ServiceClient, poolID, memberID string, opts MemberUpdateOpts) error {
	if err := requireLBClient(client); err != nil {
		return err
	}
	shared.Debugf("[lb] UpdateMember: starting, poolID=%s memberID=%s", poolID, memberID)
	_, err := pools.UpdateMember(ctx, client, poolID, memberID, memberUpdateRequest(opts)).Extract()
	if err != nil {
		shared.Debugf("[lb] UpdateMember: error: %v", err)
		return fmt.Errorf("updating member %s: %w", memberID, err)
	}
	shared.Debugf("[lb] UpdateMember: success, poolID=%s memberID=%s", poolID, memberID)
	return nil
}

// ListMembers fetches members for a pool.
func ListMembers(ctx context.Context, client *gophercloud.ServiceClient, poolID string) ([]Member, error) {
	if err := requireLBClient(client); err != nil {
		return nil, err
	}
	shared.Debugf("[lb] ListMembers: starting, poolID=%s", poolID)
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
		shared.Debugf("[lb] ListMembers: error: %v", err)
		return nil, fmt.Errorf("listing members for pool %s: %w", poolID, err)
	}
	shared.Debugf("[lb] ListMembers: success, poolID=%s count=%d", poolID, len(result))
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

// CreateHealthMonitor creates a health monitor for a pool.
func CreateHealthMonitor(ctx context.Context, client *gophercloud.ServiceClient, poolID, monType string, delay, timeout, maxRetries int, urlPath, expectedCodes, httpMethod string) (*HealthMonitor, error) {
	if err := requireLBClient(client); err != nil {
		return nil, err
	}
	shared.Debugf("[lb] CreateHealthMonitor: starting, poolID=%s type=%s", poolID, monType)
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
		shared.Debugf("[lb] CreateHealthMonitor: error: %v", err)
		return nil, fmt.Errorf("creating health monitor: %w", err)
	}
	shared.Debugf("[lb] CreateHealthMonitor: success, id=%s poolID=%s", mon.ID, poolID)
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

// UpdateHealthMonitor updates a health monitor's settings.
func UpdateHealthMonitor(ctx context.Context, client *gophercloud.ServiceClient, id string, delay, timeout, maxRetries *int, urlPath, expectedCodes, httpMethod *string) error {
	if err := requireLBClient(client); err != nil {
		return err
	}
	shared.Debugf("[lb] UpdateHealthMonitor: starting, id=%s", id)
	opts := monitors.UpdateOpts{}
	if delay != nil {
		opts.Delay = *delay
	}
	if timeout != nil {
		opts.Timeout = *timeout
	}
	if maxRetries != nil {
		opts.MaxRetries = *maxRetries
	}
	if urlPath != nil {
		opts.URLPath = *urlPath
	}
	if expectedCodes != nil {
		opts.ExpectedCodes = *expectedCodes
	}
	if httpMethod != nil {
		opts.HTTPMethod = *httpMethod
	}
	_, err := monitors.Update(ctx, client, id, opts).Extract()
	if err != nil {
		shared.Debugf("[lb] UpdateHealthMonitor: error: %v", err)
		return fmt.Errorf("updating health monitor %s: %w", id, err)
	}
	shared.Debugf("[lb] UpdateHealthMonitor: success, id=%s", id)
	return nil
}

// DeleteHealthMonitor deletes a health monitor.
func DeleteHealthMonitor(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	if err := requireLBClient(client); err != nil {
		return err
	}
	shared.Debugf("[lb] DeleteHealthMonitor: starting, id=%s", id)
	r := monitors.Delete(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[lb] DeleteHealthMonitor: error: %v", r.Err)
		return fmt.Errorf("deleting health monitor %s: %w", id, r.Err)
	}
	shared.Debugf("[lb] DeleteHealthMonitor: success, id=%s", id)
	return nil
}

// WaitForActive polls the LB until its provisioning status is ACTIVE or the
// context is cancelled. Transient polling errors are retried until the
// deadline; 404 aborts immediately, as do auth errors and ERROR statuses.
// Reaching the deadline without ACTIVE is an error.
func WaitForActive(ctx context.Context, client *gophercloud.ServiceClient, lbID string, timeout time.Duration) error {
	if err := requireLBClient(client); err != nil {
		return err
	}
	shared.Debugf("[lb] WaitForActive: starting, lbID=%s timeout=%s", lbID, timeout)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		lb, err := loadbalancers.Get(ctx, client, lbID).Extract()
		if err != nil {
			switch {
			case gophercloud.ResponseCodeIs(err, http.StatusNotFound):
				shared.Debugf("[lb] WaitForActive: load balancer not found, lbID=%s", lbID)
				return fmt.Errorf("load balancer %s not found", lbID)
			case gophercloud.ResponseCodeIs(err, http.StatusUnauthorized) || gophercloud.ResponseCodeIs(err, http.StatusForbidden):
				shared.Debugf("[lb] WaitForActive: polling error: %v", err)
				return fmt.Errorf("polling LB %s: %w", lbID, err)
			default:
				shared.Debugf("[lb] WaitForActive: transient polling error, retrying: %v", err)
			}
		} else {
			if lb.ProvisioningStatus == "ACTIVE" {
				shared.Debugf("[lb] WaitForActive: success, lbID=%s is ACTIVE", lbID)
				return nil
			}
			if strings.HasPrefix(lb.ProvisioningStatus, "ERROR") {
				shared.Debugf("[lb] WaitForActive: error status, lbID=%s status=%s", lbID, lb.ProvisioningStatus)
				return fmt.Errorf("LB %s entered %s", lbID, lb.ProvisioningStatus)
			}
		}
		select {
		case <-ctx.Done():
			shared.Debugf("[lb] WaitForActive: context cancelled, lbID=%s", lbID)
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	shared.Debugf("[lb] WaitForActive: timed out, lbID=%s", lbID)
	return fmt.Errorf("timed out waiting for LB %s to become ACTIVE", lbID)
}
