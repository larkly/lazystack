package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/groups"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/rules"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/larkly/lazystack/internal/shared"
)

// SecurityGroup is a simplified security group.
type SecurityGroup struct {
	ID          string
	Name        string
	Description string
	Rules       []SecurityRule
}

// SecurityRule is a simplified security group rule.
type SecurityRule struct {
	ID            string
	Direction     string // ingress or egress
	EtherType     string // IPv4 or IPv6
	Protocol      string
	PortRangeMin  int
	PortRangeMax  int
	RemoteIPPrefix string
	RemoteGroupID string
}

// ListSecurityGroups fetches all security groups with their rules.
func ListSecurityGroups(ctx context.Context, client *gophercloud.ServiceClient) ([]SecurityGroup, error) {
	shared.Debugf("[network] listing security groups")
	var result []SecurityGroup
	err := groups.List(client, groups.ListOpts{}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := groups.ExtractGroups(page)
		if err != nil {
			return false, err
		}
		for _, sg := range extracted {
			group := SecurityGroup{
				ID:          sg.ID,
				Name:        sg.Name,
				Description: sg.Description,
			}
			for _, r := range sg.Rules {
				group.Rules = append(group.Rules, SecurityRule{
					ID:            r.ID,
					Direction:     r.Direction,
					EtherType:     r.EtherType,
					Protocol:      r.Protocol,
					PortRangeMin:  r.PortRangeMin,
					PortRangeMax:  r.PortRangeMax,
					RemoteIPPrefix: r.RemoteIPPrefix,
					RemoteGroupID: r.RemoteGroupID,
				})
			}
			result = append(result, group)
		}
		return true, nil
	})
	if err != nil {
		shared.Debugf("[network] list security groups: %v", err)
		return nil, fmt.Errorf("listing security groups: %w", err)
	}
	shared.Debugf("[network] listed %d security groups", len(result))
	return result, nil
}

// CreateSecurityGroupRule creates a new security group rule.
func CreateSecurityGroupRule(ctx context.Context, client *gophercloud.ServiceClient, opts rules.CreateOpts) (*SecurityRule, error) {
	shared.Debugf("[network] creating security group rule (group: %s, direction: %s, protocol: %s)", opts.SecGroupID, opts.Direction, opts.Protocol)
	r := rules.Create(ctx, client, opts)
	rule, err := r.Extract()
	if err != nil {
		shared.Debugf("[network] create security group rule (group: %s): %v", opts.SecGroupID, err)
		return nil, fmt.Errorf("creating security group rule: %w", err)
	}
	shared.Debugf("[network] created security group rule %s", rule.ID)
	return &SecurityRule{
		ID:            rule.ID,
		Direction:     rule.Direction,
		EtherType:     rule.EtherType,
		Protocol:      rule.Protocol,
		PortRangeMin:  rule.PortRangeMin,
		PortRangeMax:  rule.PortRangeMax,
		RemoteIPPrefix: rule.RemoteIPPrefix,
		RemoteGroupID: rule.RemoteGroupID,
	}, nil
}

// CreateSecurityGroup creates a new security group.
func CreateSecurityGroup(ctx context.Context, client *gophercloud.ServiceClient, name, description string) (*SecurityGroup, error) {
	shared.Debugf("[network] creating security group %q", name)
	r := groups.Create(ctx, client, groups.CreateOpts{
		Name:        name,
		Description: description,
	})
	sg, err := r.Extract()
	if err != nil {
		shared.Debugf("[network] create security group %q: %v", name, err)
		return nil, fmt.Errorf("creating security group: %w", err)
	}
	shared.Debugf("[network] created security group %q (ID: %s)", sg.Name, sg.ID)
	return &SecurityGroup{
		ID:          sg.ID,
		Name:        sg.Name,
		Description: sg.Description,
	}, nil
}

// DeleteSecurityGroup deletes a security group.
func DeleteSecurityGroup(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[network] deleting security group %s", id)
	r := groups.Delete(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[network] delete security group %s: %v", id, r.Err)
		return fmt.Errorf("deleting security group %s: %w", id, r.Err)
	}
	shared.Debugf("[network] deleted security group %s", id)
	return nil
}

// DeleteSecurityGroupRule deletes a security group rule.
func DeleteSecurityGroupRule(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	shared.Debugf("[network] deleting security group rule %s", id)
	r := rules.Delete(ctx, client, id)
	if r.Err != nil {
		shared.Debugf("[network] delete security group rule %s: %v", id, r.Err)
		return fmt.Errorf("deleting security group rule %s: %w", id, r.Err)
	}
	shared.Debugf("[network] deleted security group rule %s", id)
	return nil
}

// GetSecurityGroup fetches a single security group by ID.
func GetSecurityGroup(ctx context.Context, client *gophercloud.ServiceClient, id string) (*SecurityGroup, error) {
	shared.Debugf("[network] getting security group %s", id)
	r := groups.Get(ctx, client, id)
	sg, err := r.Extract()
	if err != nil {
		shared.Debugf("[network] get security group %s: %v", id, err)
		return nil, fmt.Errorf("getting security group %s: %w", id, err)
	}
	shared.Debugf("[network] got security group %s (name: %q, %d rules)", id, sg.Name, len(sg.Rules))
	group := &SecurityGroup{
		ID:          sg.ID,
		Name:        sg.Name,
		Description: sg.Description,
	}
	for _, r := range sg.Rules {
		group.Rules = append(group.Rules, SecurityRule{
			ID:             r.ID,
			Direction:      r.Direction,
			EtherType:      r.EtherType,
			Protocol:       r.Protocol,
			PortRangeMin:   r.PortRangeMin,
			PortRangeMax:   r.PortRangeMax,
			RemoteIPPrefix: r.RemoteIPPrefix,
			RemoteGroupID:  r.RemoteGroupID,
		})
	}
	return group, nil
}

// UpdateSecurityGroup updates a security group's name and description.
func UpdateSecurityGroup(ctx context.Context, client *gophercloud.ServiceClient, id, name string, description *string) (*SecurityGroup, error) {
	shared.Debugf("[network] updating security group %s (name: %q)", id, name)
	opts := groups.UpdateOpts{
		Name:        name,
		Description: description,
	}
	r := groups.Update(ctx, client, id, opts)
	sg, err := r.Extract()
	if err != nil {
		shared.Debugf("[network] update security group %s: %v", id, err)
		return nil, fmt.Errorf("updating security group %s: %w", id, err)
	}
	shared.Debugf("[network] updated security group %s", id)
	return &SecurityGroup{
		ID:          sg.ID,
		Name:        sg.Name,
		Description: sg.Description,
	}, nil
}

// CloneSecurityGroup creates a copy of a security group with all its rules.
func CloneSecurityGroup(ctx context.Context, client *gophercloud.ServiceClient, srcID, newName, newDesc string) (*SecurityGroup, error) {
	shared.Debugf("[network] cloning security group %s as %q", srcID, newName)
	src, err := GetSecurityGroup(ctx, client, srcID)
	if err != nil {
		shared.Debugf("[network] clone security group %s: get source: %v", srcID, err)
		return nil, fmt.Errorf("cloning: %w", err)
	}
	newSG, err := CreateSecurityGroup(ctx, client, newName, newDesc)
	if err != nil {
		shared.Debugf("[network] clone security group %s: create target: %v", srcID, err)
		return nil, fmt.Errorf("cloning: %w", err)
	}
	for _, r := range src.Rules {
		// Skip default egress-allow-all rules — OpenStack creates these automatically
		if r.Direction == "egress" && r.Protocol == "" && r.RemoteIPPrefix == "" && r.RemoteGroupID == "" && r.PortRangeMin == 0 && r.PortRangeMax == 0 {
			continue
		}
		opts := rules.CreateOpts{
			SecGroupID:     newSG.ID,
			Direction:      rules.RuleDirection(r.Direction),
			EtherType:      rules.RuleEtherType(r.EtherType),
			Protocol:       rules.RuleProtocol(r.Protocol),
			PortRangeMin:   r.PortRangeMin,
			PortRangeMax:   r.PortRangeMax,
			RemoteIPPrefix: r.RemoteIPPrefix,
			RemoteGroupID:  r.RemoteGroupID,
		}
		_, err := CreateSecurityGroupRule(ctx, client, opts)
		if err != nil {
			shared.Debugf("[network] clone security group %s: clone rule: %v", srcID, err)
			return nil, fmt.Errorf("cloning rule: %w", err)
		}
	}
	shared.Debugf("[network] cloned security group %s as %q (ID: %s, %d rules)", srcID, newName, newSG.ID, len(src.Rules))
	return GetSecurityGroup(ctx, client, newSG.ID)
}
