package subnetedit

import (
	"strings"
	"testing"

	"github.com/larkly/lazystack/internal/network"
)

// Bad gateways and DNS servers are caught before any request.
func TestSubmitValidatesGatewayAndDNS(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		gateway string
		dns     string
		wantErr string // "" means the request is sent
	}{
		{name: "gateway inside", cidr: "10.0.0.0/24", gateway: "10.0.0.254"},
		{name: "gateway none", cidr: "10.0.0.0/24", gateway: "none"},
		{name: "gateway blank", cidr: "10.0.0.0/24", gateway: ""},
		{name: "v6 gateway inside", cidr: "2001:db8::/64", gateway: "2001:db8::1"},
		{name: "dns mixed families", cidr: "10.0.0.0/24", gateway: "10.0.0.1", dns: "192.0.2.53, 2001:4860:4860::8888"},
		{name: "gateway outside", cidr: "10.0.0.0/24", gateway: "10.0.1.1", wantErr: "is not inside 10.0.0.0/24"},
		{name: "gateway other family", cidr: "10.0.0.0/24", gateway: "2001:db8::1", wantErr: "is not inside 10.0.0.0/24"},
		{name: "gateway garbage", cidr: "10.0.0.0/24", gateway: "gw", wantErr: "is not a valid IP address"},
		{name: "dns garbage", cidr: "10.0.0.0/24", gateway: "10.0.0.1", dns: "192.0.2.53, dns.example.com", wantErr: `DNS server "dns.example.com" is not a valid IP address`},
		{name: "dns cidr", cidr: "10.0.0.0/24", gateway: "10.0.0.1", dns: "192.0.2.0/24", wantErr: "is not a valid IP address"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(nil, network.Subnet{ID: "sub-1", Name: "app", CIDR: tc.cidr, GatewayIP: "10.0.0.1", EnableDHCP: true})
			m.gatewayInput.SetValue(tc.gateway)
			m.dnsInput.SetValue(tc.dns)
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

// An unchanged gateway is not re-validated, so odd existing data does not
// block an unrelated edit.
func TestUnchangedGatewayNotRevalidated(t *testing.T) {
	m := New(nil, network.Subnet{ID: "sub-1", Name: "app", CIDR: "10.0.0.0/24", GatewayIP: "10.9.9.9"})
	m.nameInput.SetValue("renamed")
	m, cmd := m.submit()
	if cmd == nil || m.err != "" {
		t.Fatalf("submit rejected an unchanged gateway: %q", m.err)
	}
}
