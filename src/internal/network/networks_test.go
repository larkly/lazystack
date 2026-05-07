package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

// networksFixture is a minimal neutron API paginated network list response.
const networksFixture = `{
  "networks": [
    {
      "id": "3d7e11e8-8f4c-4b78-bd91-06cdab3aab91",
      "name": "private-net",
      "status": "ACTIVE",
      "shared": true,
      "subnets": [
        "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
        "b2c3d4e5-f6a7-8901-bcde-f12345678901"
      ]
    },
    {
      "id": "7aa3b4a5-6c7d-4e8f-9a0b-1c2d3e4f5a6b",
      "name": "public-net",
      "status": "DOWN",
      "shared": false,
      "subnets": []
    }
  ]
}`

// subnetsFixture is a minimal neutron API paginated subnet list response with
// nested allocation_pools and host_routes.
const subnetsFixture = `{
  "subnets": [
    {
      "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "name": "subnet-v4",
      "network_id": "3d7e11e8-8f4c-4b78-bd91-06cdab3aab91",
      "cidr": "10.0.0.0/24",
      "gateway_ip": "10.0.0.1",
      "ip_version": 4,
      "enable_dhcp": true,
      "allocation_pools": [
        {"start": "10.0.0.100", "end": "10.0.0.200"}
      ],
      "host_routes": [
        {"destination": "172.16.0.0/16", "nexthop": "10.0.0.254"}
      ],
      "dns_nameservers": ["8.8.8.8", "8.8.4.4"]
    },
    {
      "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
      "name": "subnet-v6",
      "network_id": "3d7e11e8-8f4c-4b78-bd91-06cdab3aab91",
      "cidr": "fd00::/64",
      "gateway_ip": "fd00::1",
      "ip_version": 6,
      "enable_dhcp": false,
      "ipv6_address_mode": "slaac",
      "allocation_pools": [],
      "host_routes": [],
      "dns_nameservers": []
    }
  ]
}`

func fakeNeutronClientNetworks(handler http.Handler) *gophercloud.ServiceClient {
	srv := httptest.NewServer(handler)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			HTTPClient: *srv.Client(),
		},
		Endpoint: srv.URL + "/",
	}
}

func TestListNetworks(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "networks") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(networksFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeNeutronClientNetworks(handler)
	ctx := context.Background()

	nets, err := ListNetworks(ctx, client)
	if err != nil {
		t.Fatalf("ListNetworks() error: %v", err)
	}
	if len(nets) != 2 {
		t.Fatalf("expected 2 networks, got %d", len(nets))
	}

	n1 := nets[0]
	if n1.ID != "3d7e11e8-8f4c-4b78-bd91-06cdab3aab91" {
		t.Errorf("unexpected ID: %s", n1.ID)
	}
	if n1.Name != "private-net" {
		t.Errorf("unexpected Name: %s", n1.Name)
	}
	if n1.Status != "ACTIVE" {
		t.Errorf("unexpected Status: %s", n1.Status)
	}
	if !n1.Shared {
		t.Errorf("expected Shared=true, got %v", n1.Shared)
	}
	if len(n1.SubnetIDs) != 2 {
		t.Errorf("expected 2 SubnetIDs, got %d", len(n1.SubnetIDs))
	}
	if n1.SubnetIDs[0] != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("unexpected first subnet ID: %s", n1.SubnetIDs[0])
	}

	n2 := nets[1]
	if n2.ID != "7aa3b4a5-6c7d-4e8f-9a0b-1c2d3e4f5a6b" {
		t.Errorf("unexpected ID: %s", n2.ID)
	}
	if n2.Name != "public-net" {
		t.Errorf("unexpected Name: %s", n2.Name)
	}
	if n2.Status != "DOWN" {
		t.Errorf("unexpected Status: %s", n2.Status)
	}
	if n2.Shared {
		t.Errorf("expected Shared=false, got true")
	}
}

func TestListSubnets(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "subnets") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(subnetsFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeNeutronClientNetworks(handler)
	ctx := context.Background()

	subs, err := ListSubnets(ctx, client)
	if err != nil {
		t.Fatalf("ListSubnets() error: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 subnets, got %d", len(subs))
	}

	s1 := subs[0]
	if s1.ID != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("unexpected ID: %s", s1.ID)
	}
	if s1.Name != "subnet-v4" {
		t.Errorf("unexpected Name: %s", s1.Name)
	}
	if s1.NetworkID != "3d7e11e8-8f4c-4b78-bd91-06cdab3aab91" {
		t.Errorf("unexpected NetworkID: %s", s1.NetworkID)
	}
	if s1.CIDR != "10.0.0.0/24" {
		t.Errorf("unexpected CIDR: %s", s1.CIDR)
	}
	if s1.GatewayIP != "10.0.0.1" {
		t.Errorf("unexpected GatewayIP: %s", s1.GatewayIP)
	}
	if s1.IPVersion != 4 {
		t.Errorf("unexpected IPVersion: %d", s1.IPVersion)
	}
	if !s1.EnableDHCP {
		t.Errorf("expected EnableDHCP=true")
	}

	// Verify nested AllocationPools
	if len(s1.AllocationPools) != 1 {
		t.Fatalf("expected 1 AllocationPool, got %d", len(s1.AllocationPools))
	}
	if s1.AllocationPools[0].Start != "10.0.0.100" {
		t.Errorf("unexpected pool Start: %s", s1.AllocationPools[0].Start)
	}
	if s1.AllocationPools[0].End != "10.0.0.200" {
		t.Errorf("unexpected pool End: %s", s1.AllocationPools[0].End)
	}

	// Verify nested HostRoutes
	if len(s1.HostRoutes) != 1 {
		t.Fatalf("expected 1 HostRoute, got %d", len(s1.HostRoutes))
	}
	if s1.HostRoutes[0].DestinationCIDR != "172.16.0.0/16" {
		t.Errorf("unexpected route DestinationCIDR: %s", s1.HostRoutes[0].DestinationCIDR)
	}
	if s1.HostRoutes[0].NextHop != "10.0.0.254" {
		t.Errorf("unexpected route NextHop: %s", s1.HostRoutes[0].NextHop)
	}

	// Verify DNSNameservers
	if len(s1.DNSNameservers) != 2 {
		t.Errorf("expected 2 DNS nameservers, got %d", len(s1.DNSNameservers))
	}

	// Verify IPv6 subnet
	s2 := subs[1]
	if s2.IPVersion != 6 {
		t.Errorf("expected IPVersion=6, got %d", s2.IPVersion)
	}
	if s2.IPv6AddressMode != "slaac" {
		t.Errorf("unexpected IPv6AddressMode: %s", s2.IPv6AddressMode)
	}
	if s2.EnableDHCP {
		t.Errorf("expected EnableDHCP=false for IPv6 subnet")
	}
}
