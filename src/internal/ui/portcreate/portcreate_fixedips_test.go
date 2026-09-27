package portcreate

import (
	"strings"
	"testing"

	"github.com/larkly/lazystack/internal/network"
)

func TestParseFixedIPs(t *testing.T) {
	single4 := Model{subnets: []network.Subnet{{ID: "11111111-aaaa", Name: "v4", CIDR: "10.0.0.0/24", IPVersion: 4}}}
	single6 := Model{subnets: []network.Subnet{{ID: "66666666-bbbb", Name: "v6", CIDR: "2001:db8::/64", IPVersion: 6}}}
	dual := Model{subnets: []network.Subnet{
		{ID: "11111111-aaaa", Name: "v4", CIDR: "10.0.0.0/24", IPVersion: 4},
		{ID: "11112222-cccc", Name: "v4b", CIDR: "10.9.0.0/24", IPVersion: 4},
		{ID: "66666666-bbbb", Name: "v6", CIDR: "2001:db8::/64", IPVersion: 6},
		{ID: "77777777-dddd", Name: "dup", CIDR: "10.1.0.0/24", IPVersion: 4},
		{ID: "88888888-eeee", Name: "dup", CIDR: "10.2.0.0/24", IPVersion: 4},
	}}
	ok := []struct {
		m    Model
		raw  string
		want string
	}{
		{single4, "10.0.0.5", "11111111-aaaa=10.0.0.5"},
		{single6, "2001:db8::5", "66666666-bbbb=2001:db8::5"},
		{single6, "v6:2001:db8::5", "66666666-bbbb=2001:db8::5"},
		{dual, "v6:2001:db8::7, v4:10.0.0.9", "66666666-bbbb=2001:db8::7,11111111-aaaa=10.0.0.9"},
		{dual, "11111111-aaaa:10.0.0.9", "11111111-aaaa=10.0.0.9"},
		{dual, "66666666-b:2001:db8::8", "66666666-bbbb=2001:db8::8"}, // ID prefix
		{dual, "2001:db8::9", "66666666-bbbb=2001:db8::9"},
		{dual, "v4:", "11111111-aaaa="},
		{dual, "10.0.0.5", "11111111-aaaa=10.0.0.5"}, // the only subnet containing it
	}
	for _, c := range ok {
		got, err := c.m.parseFixedIPs(c.raw)
		if err != nil {
			t.Errorf("parseFixedIPs(%q): %v", c.raw, err)
			continue
		}
		var parts []string
		for _, f := range got {
			parts = append(parts, f.SubnetID+"="+f.IPAddress)
		}
		if strings.Join(parts, ",") != c.want {
			t.Errorf("parseFixedIPs(%q) = %v, want %s", c.raw, parts, c.want)
		}
	}
	bad := []struct {
		m   Model
		raw string
	}{
		{single4, ":10.0.0.2"},
		{dual, "1111:10.0.0.2"},     // ambiguous ID prefix
		{dual, "dup:10.1.0.5"},      // ambiguous name
		{dual, "nope:10.0.0.2"},     // unknown subnet
		{dual, "v4:not-an-ip"},      // invalid address
		{dual, "192.168.1.1"},       // no subnet contains it
		{Model{}, "10.0.0.5"},       // no subnets
		{single4, "v4:10.0.0.2:99"}, // garbage
	}
	for _, c := range bad {
		if got, err := c.m.parseFixedIPs(c.raw); err == nil {
			t.Errorf("parseFixedIPs(%q) = %v, want error", c.raw, got)
		}
	}
}
