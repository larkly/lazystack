package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

// routersFixture is a minimal neutron API paginated router list response with
// nested external_fixed_ips in gateway_info and routes.
const routersFixture = `{
  "routers": [
    {
      "id": "e5f6a7b8-c9d0-1234-5678-90abcdef1234",
      "name": "router-dual",
      "description": "Dual-stack gateway router",
      "status": "ACTIVE",
      "admin_state_up": true,
      "external_gateway_info": {
        "network_id": "7aa3b4a5-6c7d-4e8f-9a0b-1c2d3e4f5a6b",
        "external_fixed_ips": [
          {"ip_address": "203.0.113.10", "subnet_id": "f001"},
          {"ip_address": "2001:db8::10", "subnet_id": "f002"}
        ]
      },
      "routes": [
        {"destination": "10.1.0.0/16", "nexthop": "10.0.0.254"}
      ]
    },
    {
      "id": "f6a7b8c9-d0e1-2345-6789-0abcdef12345",
      "name": "router-v4-only",
      "status": "DOWN",
      "admin_state_up": false,
      "external_gateway_info": {
        "network_id": "7aa3b4a5-6c7d-4e8f-9a0b-1c2d3e4f5a6b",
        "external_fixed_ips": [
          {"ip_address": "203.0.113.11", "subnet_id": "f001"}
        ]
      },
      "routes": []
    }
  ]
}`

// getRouterFixture is a single-router detail response (non-paginated GET).
const getRouterFixture = `{
  "router": {
    "id": "e5f6a7b8-c9d0-1234-5678-90abcdef1234",
    "name": "router-dual",
    "description": "Dual-stack gateway router",
    "status": "ACTIVE",
    "admin_state_up": true,
    "external_gateway_info": {
      "network_id": "7aa3b4a5-6c7d-4e8f-9a0b-1c2d3e4f5a6b",
      "external_fixed_ips": [
        {"ip_address": "203.0.113.10", "subnet_id": "f001"},
        {"ip_address": "2001:db8::10", "subnet_id": "f002"}
      ]
    },
    "routes": [
      {"destination": "10.1.0.0/16", "nexthop": "10.0.0.254"},
      {"destination": "192.168.0.0/24", "nexthop": "10.0.0.253"}
    ]
  }
}`

func fakeNeutronClientRouters(handler http.Handler) *gophercloud.ServiceClient {
	srv := httptest.NewServer(handler)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			HTTPClient: *srv.Client(),
		},
		Endpoint: srv.URL + "/",
	}
}

func TestListRouters(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "routers") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(routersFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeNeutronClientRouters(handler)
	ctx := context.Background()

	routers, err := ListRouters(ctx, client)
	if err != nil {
		t.Fatalf("ListRouters() error: %v", err)
	}
	if len(routers) != 2 {
		t.Fatalf("expected 2 routers, got %d", len(routers))
	}

	// Router 1: dual-stack with IPv4 and IPv6 external IPs + routes
	r1 := routers[0]
	if r1.ID != "e5f6a7b8-c9d0-1234-5678-90abcdef1234" {
		t.Errorf("unexpected ID: %s", r1.ID)
	}
	if r1.Name != "router-dual" {
		t.Errorf("unexpected Name: %s", r1.Name)
	}
	if r1.Description != "Dual-stack gateway router" {
		t.Errorf("unexpected Description: %s", r1.Description)
	}
	if r1.Status != "ACTIVE" {
		t.Errorf("unexpected Status: %s", r1.Status)
	}
	if !r1.AdminStateUp {
		t.Errorf("expected AdminStateUp=true")
	}
	if r1.ExternalGatewayNetworkID != "7aa3b4a5-6c7d-4e8f-9a0b-1c2d3e4f5a6b" {
		t.Errorf("unexpected ExternalGatewayNetworkID: %s", r1.ExternalGatewayNetworkID)
	}
	if r1.ExternalGatewayIPv4 != "203.0.113.10" {
		t.Errorf("unexpected ExternalGatewayIPv4: %s", r1.ExternalGatewayIPv4)
	}
	if r1.ExternalGatewayIPv6 != "2001:db8::10" {
		t.Errorf("unexpected ExternalGatewayIPv6: %s", r1.ExternalGatewayIPv6)
	}
	if len(r1.Routes) != 1 {
		t.Fatalf("expected 1 Route, got %d", len(r1.Routes))
	}
	if r1.Routes[0].DestinationCIDR != "10.1.0.0/16" {
		t.Errorf("unexpected route DestinationCIDR: %s", r1.Routes[0].DestinationCIDR)
	}
	if r1.Routes[0].NextHop != "10.0.0.254" {
		t.Errorf("unexpected route NextHop: %s", r1.Routes[0].NextHop)
	}

	// Router 2: IPv4-only, DOWN, admin state down, no routes
	r2 := routers[1]
	if r2.ID != "f6a7b8c9-d0e1-2345-6789-0abcdef12345" {
		t.Errorf("unexpected ID: %s", r2.ID)
	}
	if r2.Name != "router-v4-only" {
		t.Errorf("unexpected Name: %s", r2.Name)
	}
	if r2.Status != "DOWN" {
		t.Errorf("unexpected Status: %s", r2.Status)
	}
	if r2.AdminStateUp {
		t.Errorf("expected AdminStateUp=false")
	}
	if r2.ExternalGatewayIPv4 != "203.0.113.11" {
		t.Errorf("unexpected ExternalGatewayIPv4: %s", r2.ExternalGatewayIPv4)
	}
	if r2.ExternalGatewayIPv6 != "" {
		t.Errorf("expected empty ExternalGatewayIPv6, got %s", r2.ExternalGatewayIPv6)
	}
	if len(r2.Routes) != 0 {
		t.Errorf("expected 0 Routes, got %d", len(r2.Routes))
	}
}

func TestGetRouter(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "routers") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(getRouterFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeNeutronClientRouters(handler)
	ctx := context.Background()

	router, err := GetRouter(ctx, client, "e5f6a7b8-c9d0-1234-5678-90abcdef1234")
	if err != nil {
		t.Fatalf("GetRouter() error: %v", err)
	}
	if router == nil {
		t.Fatal("expected non-nil router")
	}

	if router.ID != "e5f6a7b8-c9d0-1234-5678-90abcdef1234" {
		t.Errorf("unexpected ID: %s", router.ID)
	}
	if router.Name != "router-dual" {
		t.Errorf("unexpected Name: %s", router.Name)
	}
	if router.Description != "Dual-stack gateway router" {
		t.Errorf("unexpected Description: %s", router.Description)
	}
	if router.Status != "ACTIVE" {
		t.Errorf("unexpected Status: %s", router.Status)
	}
	if !router.AdminStateUp {
		t.Error("expected AdminStateUp=true")
	}
	if router.ExternalGatewayNetworkID != "7aa3b4a5-6c7d-4e8f-9a0b-1c2d3e4f5a6b" {
		t.Errorf("unexpected ExternalGatewayNetworkID: %s", router.ExternalGatewayNetworkID)
	}
	if router.ExternalGatewayIPv4 != "203.0.113.10" {
		t.Errorf("unexpected ExternalGatewayIPv4: %s", router.ExternalGatewayIPv4)
	}
	if router.ExternalGatewayIPv6 != "2001:db8::10" {
		t.Errorf("unexpected ExternalGatewayIPv6: %s", router.ExternalGatewayIPv6)
	}

	// Verify both routes from the detail response
	if len(router.Routes) != 2 {
		t.Fatalf("expected 2 Routes in detail view, got %d", len(router.Routes))
	}
	if router.Routes[0].DestinationCIDR != "10.1.0.0/16" {
		t.Errorf("unexpected route[0] DestinationCIDR: %s", router.Routes[0].DestinationCIDR)
	}
	if router.Routes[0].NextHop != "10.0.0.254" {
		t.Errorf("unexpected route[0] NextHop: %s", router.Routes[0].NextHop)
	}
	if router.Routes[1].DestinationCIDR != "192.168.0.0/24" {
		t.Errorf("unexpected route[1] DestinationCIDR: %s", router.Routes[1].DestinationCIDR)
	}
	if router.Routes[1].NextHop != "10.0.0.253" {
		t.Errorf("unexpected route[1] NextHop: %s", router.Routes[1].NextHop)
	}
}
