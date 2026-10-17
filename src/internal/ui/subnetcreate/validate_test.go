package subnetcreate

import (
	"strings"
	"testing"

	"github.com/larkly/lazystack/internal/network"
)

// Bad CIDRs and gateways are caught before any request, so Neutron's raw
// 400 never reaches the user.
func TestSubmitValidatesCIDRAndGateway(t *testing.T) {
	tests := []struct {
		name    string
		ipv6    bool
		pool    bool
		cidr    string
		gateway string
		wantErr string // "" means the request is sent
	}{
		{name: "v4 auto gateway", cidr: "10.0.0.0/24"},
		{name: "v4 none", cidr: "10.0.0.0/24", gateway: "none"},
		{name: "v4 gateway inside", cidr: "10.0.0.0/24", gateway: "10.0.0.1"},
		{name: "v6 gateway inside", ipv6: true, cidr: "2001:db8::/64", gateway: "2001:db8::1"},
		{name: "v6 NONE", ipv6: true, cidr: "2001:db8::/64", gateway: "NONE"},
		{name: "pool v4 gateway", pool: true, gateway: "10.0.0.1"},
		{name: "garbage CIDR", cidr: "10.0.0/24", wantErr: "is not a valid CIDR"},
		{name: "bare address", cidr: "10.0.0.0", wantErr: "is not a valid CIDR"},
		{name: "v6 CIDR with IPv4", cidr: "2001:db8::/64", wantErr: "is IPv6 but IPv4 is selected"},
		{name: "v4 CIDR with IPv6", ipv6: true, cidr: "10.0.0.0/24", wantErr: "is IPv4 but IPv6 is selected"},
		{name: "host bits", cidr: "10.0.0.5/24", wantErr: "use 10.0.0.0/24"},
		{name: "gateway outside", cidr: "10.0.0.0/24", gateway: "10.0.1.1", wantErr: "is not inside 10.0.0.0/24"},
		{name: "gateway garbage", cidr: "10.0.0.0/24", gateway: "router", wantErr: "is not a valid IP address"},
		{name: "gateway other family", ipv6: true, cidr: "2001:db8::/64", gateway: "10.0.0.1", wantErr: "is not inside 2001:db8::/64"},
		{name: "pool gateway other family", pool: true, gateway: "2001:db8::1", wantErr: "is IPv6 but IPv4 is selected"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(nil, "net", "net")
			if tc.ipv6 {
				m.ipVersion = 1
			}
			if tc.pool {
				m.subnetPools = []network.SubnetPool{{ID: "pool-1", Name: "pool"}}
				m.subnetPool = 1
			}
			m.cidrInput.SetValue(tc.cidr)
			m.gatewayInput.SetValue(tc.gateway)
			m, cmd := m.submit()
			if tc.wantErr == "" {
				if cmd == nil || m.err != "" {
					t.Fatalf("submit rejected valid input: %q", m.err)
				}
				return
			}
			if cmd != nil {
				t.Fatalf("submit sent a request, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(m.err, tc.wantErr) {
				t.Fatalf("err = %q, want it to contain %q", m.err, tc.wantErr)
			}
		})
	}
}
