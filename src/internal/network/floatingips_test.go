package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

// floatingIPsFixture is a minimal neutron API paginated response.
const floatingIPsFixture = `{
  "floatingips": [
    {
      "id": "d4e1e3b0-3d79-4c91-93c7-c9da4d9b3c10",
      "floating_ip_address": "203.0.113.5",
      "fixed_ip_address": "10.0.0.4",
      "floating_network_id": "a6917946-38ab-4ffd-a55a-26c0980ce5ee",
      "port_id": "8b9b7c5b-d56b-4b57-a1c0-5f7c6a1ae22e",
      "tenant_id": "7c6e4f8b1a2d3c5e6f7a8b9c0d1e2f3a",
      "status": "ACTIVE",
      "router_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "project_id": "7c6e4f8b1a2d3c5e6f7a8b9c0d1e2f3a"
    },
    {
      "id": "c9b8a7d6-e5f4-3210-abcd-ef9876543210",
      "floating_ip_address": "203.0.113.6",
      "floating_network_id": "a6917946-38ab-4ffd-a55a-26c0980ce5ee",
      "tenant_id": "7c6e4f8b1a2d3c5e6f7a8b9c0d1e2f3a",
      "status": "DOWN",
      "project_id": "7c6e4f8b1a2d3c5e6f7a8b9c0d1e2f3a"
    }
  ]
}`

// allocateFixture is a minimal neutron create floating IP response.
const allocateFixture = `{
  "floatingip": {
    "id": "d4e1e3b0-3d79-4c91-93c7-c9da4d9b3c10",
    "floating_ip_address": "203.0.113.5",
    "floating_network_id": "a6917946-38ab-4ffd-a55a-26c0980ce5ee",
    "status": "ACTIVE",
    "tenant_id": "7c6e4f8b1a2d3c5e6f7a8b9c0d1e2f3a",
    "project_id": "7c6e4f8b1a2d3c5e6f7a8b9c0d1e2f3a"
  }
}`

func fakeNeutronClient(handler http.Handler) *gophercloud.ServiceClient {
	srv := httptest.NewServer(handler)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			HTTPClient: *srv.Client(),
		},
		Endpoint: srv.URL + "/",
	}
}

func TestListFloatingIPs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "floatingips") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(floatingIPsFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeNeutronClient(handler)
	ctx := context.Background()

	fips, err := ListFloatingIPs(ctx, client)
	if err != nil {
		t.Fatalf("ListFloatingIPs() error: %v", err)
	}
	if len(fips) != 2 {
		t.Fatalf("expected 2 floating IPs, got %d", len(fips))
	}

	fip1 := fips[0]
	if fip1.ID != "d4e1e3b0-3d79-4c91-93c7-c9da4d9b3c10" {
		t.Errorf("unexpected ID: %s", fip1.ID)
	}
	if fip1.FloatingIP != "203.0.113.5" {
		t.Errorf("unexpected floating IP address: %s", fip1.FloatingIP)
	}
	if fip1.FixedIP != "10.0.0.4" {
		t.Errorf("unexpected fixed IP: %s", fip1.FixedIP)
	}
	if fip1.Status != "ACTIVE" {
		t.Errorf("unexpected status: %s", fip1.Status)
	}

	fip2 := fips[1]
	if fip2.FloatingIP != "203.0.113.6" {
		t.Errorf("unexpected second floating IP: %s", fip2.FloatingIP)
	}
	if fip2.Status != "DOWN" {
		t.Errorf("unexpected second status: %s", fip2.Status)
	}
}

func TestAllocateFloatingIP(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "floatingips") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(allocateFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeNeutronClient(handler)
	ctx := context.Background()

	fip, err := AllocateFloatingIP(ctx, client, "a6917946-38ab-4ffd-a55a-26c0980ce5ee")
	if err != nil {
		t.Fatalf("AllocateFloatingIP() error: %v", err)
	}
	if fip == nil {
		t.Fatal("expected non-nil floating IP")
	}
	if fip.ID != "d4e1e3b0-3d79-4c91-93c7-c9da4d9b3c10" {
		t.Errorf("unexpected ID: %s", fip.ID)
	}
	if fip.FloatingIP != "203.0.113.5" {
		t.Errorf("unexpected floating IP address: %s", fip.FloatingIP)
	}
	if fip.FloatingNetworkID != "a6917946-38ab-4ffd-a55a-26c0980ce5ee" {
		t.Errorf("unexpected network ID: %s", fip.FloatingNetworkID)
	}
	if fip.Status != "ACTIVE" {
		t.Errorf("unexpected status: %s", fip.Status)
	}
}
