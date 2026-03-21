package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/groups"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/rules"
	"github.com/gophercloud/gophercloud/v2/pagination"
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
		return nil, fmt.Errorf("listing security groups: %w", err)
	}
	return result, nil
}

// CreateSecurityGroupRule creates a new security group rule.
func CreateSecurityGroupRule(ctx context.Context, client *gophercloud.ServiceClient, opts rules.CreateOpts) (*SecurityRule, error) {
	r := rules.Create(ctx, client, opts)
	rule, err := r.Extract()
	if err != nil {
		return nil, fmt.Errorf("creating security group rule: %w", err)
	}
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

// DeleteSecurityGroupRule deletes a security group rule.
func DeleteSecurityGroupRule(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	r := rules.Delete(ctx, client, id)
	if r.Err != nil {
		return fmt.Errorf("deleting security group rule %s: %w", id, r.Err)
	}
	return nil
}
