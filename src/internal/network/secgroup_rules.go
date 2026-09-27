package network

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/rules"
	"github.com/larkly/lazystack/internal/shared"
)

// SecurityRuleSpec is the lossless definition of a security group rule, as
// needed to recreate it exactly. Nil port bounds mean "unset" (any port, or
// any ICMP type/code), which differs from an explicit 0 such as ICMP type 0
// (echo reply). An empty Protocol means any protocol. At most one of the
// remote fields is set.
type SecurityRuleSpec struct {
	ID                   string
	SecGroupID           string
	Direction            string
	EtherType            string
	Protocol             string
	PortRangeMin         *int
	PortRangeMax         *int
	RemoteIPPrefix       string
	RemoteGroupID        string
	RemoteAddressGroupID string
	Description          string
}

// ruleSpecWire mirrors the Neutron rule representation with nullable
// fields; the SDK result type collapses null ports into 0.
type ruleSpecWire struct {
	ID                   string  `json:"id"`
	SecGroupID           string  `json:"security_group_id"`
	Direction            string  `json:"direction"`
	EtherType            string  `json:"ethertype"`
	Protocol             *string `json:"protocol"`
	PortRangeMin         *int    `json:"port_range_min"`
	PortRangeMax         *int    `json:"port_range_max"`
	RemoteIPPrefix       *string `json:"remote_ip_prefix"`
	RemoteGroupID        *string `json:"remote_group_id"`
	RemoteAddressGroupID *string `json:"remote_address_group_id"`
	Description          *string `json:"description"`
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// GetSecurityRuleSpec fetches a single rule with null-aware fields.
func GetSecurityRuleSpec(ctx context.Context, client *gophercloud.ServiceClient, id string) (*SecurityRuleSpec, error) {
	shared.Debugf("[network] getting security group rule %s", id)
	var body struct {
		Rule ruleSpecWire `json:"security_group_rule"`
	}
	if err := rules.Get(ctx, client, id).ExtractInto(&body); err != nil {
		shared.Debugf("[network] get security group rule %s: %v", id, err)
		return nil, fmt.Errorf("getting security group rule %s: %w", id, err)
	}
	w := body.Rule
	return &SecurityRuleSpec{
		ID:                   w.ID,
		SecGroupID:           w.SecGroupID,
		Direction:            w.Direction,
		EtherType:            w.EtherType,
		Protocol:             derefString(w.Protocol),
		PortRangeMin:         w.PortRangeMin,
		PortRangeMax:         w.PortRangeMax,
		RemoteIPPrefix:       derefString(w.RemoteIPPrefix),
		RemoteGroupID:        derefString(w.RemoteGroupID),
		RemoteAddressGroupID: derefString(w.RemoteAddressGroupID),
		Description:          derefString(w.Description),
	}, nil
}

// ruleSpecCreate builds a create request that keeps explicit zero port
// bounds (the SDK's CreateOpts drops them via omitempty).
type ruleSpecCreate SecurityRuleSpec

func (b ruleSpecCreate) ToSecGroupRuleCreateMap() (map[string]any, error) {
	r := map[string]any{
		"security_group_id": b.SecGroupID,
		"direction":         b.Direction,
		"ethertype":         b.EtherType,
	}
	if b.Protocol != "" {
		r["protocol"] = b.Protocol
	}
	if b.PortRangeMin != nil {
		r["port_range_min"] = *b.PortRangeMin
	}
	if b.PortRangeMax != nil {
		r["port_range_max"] = *b.PortRangeMax
	}
	if b.RemoteIPPrefix != "" {
		r["remote_ip_prefix"] = b.RemoteIPPrefix
	}
	if b.RemoteGroupID != "" {
		r["remote_group_id"] = b.RemoteGroupID
	}
	if b.RemoteAddressGroupID != "" {
		r["remote_address_group_id"] = b.RemoteAddressGroupID
	}
	if b.Description != "" {
		r["description"] = b.Description
	}
	return map[string]any{"security_group_rule": r}, nil
}

// CreateSecurityRuleSpec creates a rule exactly as described by spec (its ID
// is ignored) and returns the new rule's ID.
func CreateSecurityRuleSpec(ctx context.Context, client *gophercloud.ServiceClient, spec SecurityRuleSpec) (string, error) {
	shared.Debugf("[network] creating security group rule (group: %s, direction: %s, protocol: %q)", spec.SecGroupID, spec.Direction, spec.Protocol)
	remotes := 0
	for _, r := range []string{spec.RemoteIPPrefix, spec.RemoteGroupID, spec.RemoteAddressGroupID} {
		if r != "" {
			remotes++
		}
	}
	if remotes > 1 {
		return "", fmt.Errorf("creating security group rule: remote IP prefix, remote group and remote address group are mutually exclusive")
	}
	rule, err := rules.Create(ctx, client, ruleSpecCreate(spec)).Extract()
	if err != nil {
		shared.Debugf("[network] create security group rule (group: %s): %v", spec.SecGroupID, err)
		return "", fmt.Errorf("creating security group rule: %w", err)
	}
	shared.Debugf("[network] created security group rule %s", rule.ID)
	return rule.ID, nil
}
