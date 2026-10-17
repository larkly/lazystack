package network

import (
	"context"
	"strings"
	"testing"
)

// gophercloud panics on a nil *ServiceClient; clouds without Neutron must
// get an error instead.
func TestNilNetworkClientIsAnError(t *testing.T) {
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"ListFloatingIPs":         func() error { _, err := ListFloatingIPs(ctx, nil); return err },
		"ListFloatingIPTargets":   func() error { _, err := ListFloatingIPTargets(ctx, nil, "vm"); return err },
		"ListNetworks":            func() error { _, err := ListNetworks(ctx, nil); return err },
		"ListSubnets":             func() error { _, err := ListSubnets(ctx, nil); return err },
		"ListSubnetPools":         func() error { _, err := ListSubnetPools(ctx, nil); return err },
		"ListExternalNetworks":    func() error { _, err := ListExternalNetworks(ctx, nil); return err },
		"GetPort":                 func() error { _, err := GetPort(ctx, nil, "p"); return err },
		"FindRouterPortOnNetwork": func() error { _, err := FindRouterPortOnNetwork(ctx, nil, "r", "n"); return err },
		"ListPortsByDevice":       func() error { _, err := ListPortsByDevice(ctx, nil, "d"); return err },
		"ListPortsBySecurityGroup": func() error {
			_, err := ListPortsBySecurityGroup(ctx, nil, "sg")
			return err
		},
		"ListPorts":            func() error { _, err := ListPorts(ctx, nil, "n"); return err },
		"ListRouters":          func() error { _, err := ListRouters(ctx, nil); return err },
		"GetRouter":            func() error { _, err := GetRouter(ctx, nil, "r"); return err },
		"ListRouterInterfaces": func() error { _, err := ListRouterInterfaces(ctx, nil, "r"); return err },
		"GetSecurityRuleSpec":  func() error { _, err := GetSecurityRuleSpec(ctx, nil, "rule"); return err },
		"ListSecurityGroups":   func() error { _, err := ListSecurityGroups(ctx, nil); return err },
		"GetSecurityGroup":     func() error { _, err := GetSecurityGroup(ctx, nil, "sg"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || !strings.Contains(err.Error(), "networking (Neutron) service is not available") {
				t.Errorf("err=%v", err)
			}
		})
	}
}
