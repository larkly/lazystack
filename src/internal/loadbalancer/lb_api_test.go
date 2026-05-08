package loadbalancer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

// lbListFixture is an Octavia v2 load balancer list response.
const lbListFixture = `{
  "loadbalancers": [
    {
      "id": "f1b9d8c2-7a3e-4d1f-92c5-e81a2d3b4f76",
      "name": "frontend-lb",
      "description": "External-facing load balancer",
      "provisioning_status": "ACTIVE",
      "operating_status": "ONLINE",
      "admin_state_up": true,
      "vip_address": "10.20.1.50",
      "vip_subnet_id": "3c8e9f2a-4b1d-4a7c-9d3e-2f1a8b6c4d70",
      "provider": "amphora",
      "tags": ["production", "frontend"],
      "created_at": "2026-03-15T08:00:00Z",
      "updated_at": "2026-05-01T12:30:00Z"
    },
    {
      "id": "a2c7e6d4-8f5b-4e9a-8c2f-7b1e3d5a9c28",
      "name": "backend-lb",
      "description": "Internal backend load balancer",
      "provisioning_status": "PENDING_CREATE",
      "operating_status": "OFFLINE",
      "admin_state_up": false,
      "vip_address": "10.20.2.100",
      "vip_subnet_id": "6f1a3b2e-9c7d-4e8f-2a5b-1d3e7f9b4c18",
      "provider": "amphora",
      "tags": ["internal", "backend"],
      "created_at": "2026-05-07T09:00:00Z",
      "updated_at": "2026-05-07T09:00:00Z"
    }
  ]
}`

// listenerListFixture is an Octavia v2 listener list response.
const listenerListFixture = `{
  "listeners": [
    {
      "id": "d3e4f5a1-b2c3-4d5e-6f7a-8b9c0d1e2f3a",
      "name": "https-listener",
      "description": "HTTPS frontend listener",
      "protocol": "HTTPS",
      "protocol_port": 443,
      "default_pool_id": "e5f6a7b2-c3d4-4e5f-6a7b-8c9d0e1f2a3b",
      "connection_limit": 1000,
      "admin_state_up": true,
      "loadbalancers": [
        {"id": "f1b9d8c2-7a3e-4d1f-92c5-e81a2d3b4f76"}
      ],
      "tags": ["tls"],
      "created_at": "2026-03-15T08:30:00Z",
      "updated_at": "2026-04-10T14:00:00Z"
    },
    {
      "id": "b4c5d6e1-f2a3-4b5c-6d7e-8f9a0b1c2d3e",
      "name": "http-listener",
      "description": "HTTP redirect listener",
      "protocol": "HTTP",
      "protocol_port": 80,
      "default_pool_id": "",
      "connection_limit": -1,
      "admin_state_up": true,
      "loadbalancers": [
        {"id": "f1b9d8c2-7a3e-4d1f-92c5-e81a2d3b4f76"}
      ],
      "tags": [],
      "created_at": "2026-03-15T08:30:00Z",
      "updated_at": "2026-03-15T08:30:00Z"
    }
  ]
}`

func fakeLBClient(handler http.Handler) *gophercloud.ServiceClient {
	srv := httptest.NewServer(handler)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			HTTPClient: *srv.Client(),
		},
		Endpoint: srv.URL + "/",
	}
}

func TestListLoadBalancers(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/loadbalancers") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(lbListFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeLBClient(handler)
	ctx := context.Background()

	lbs, err := ListLoadBalancers(ctx, client)
	if err != nil {
		t.Fatalf("ListLoadBalancers() error: %v", err)
	}
	if len(lbs) != 2 {
		t.Fatalf("expected 2 LBs, got %d", len(lbs))
	}

	// Verify first LB (active)
	lb1 := lbs[0]
	if lb1.ID != "f1b9d8c2-7a3e-4d1f-92c5-e81a2d3b4f76" {
		t.Errorf("unexpected ID: %s", lb1.ID)
	}
	if lb1.Name != "frontend-lb" {
		t.Errorf("unexpected Name: %s", lb1.Name)
	}
	if lb1.VipAddress != "10.20.1.50" {
		t.Errorf("unexpected VipAddress: %s", lb1.VipAddress)
	}
	if lb1.ProvisioningStatus != "ACTIVE" {
		t.Errorf("unexpected ProvisioningStatus: %s", lb1.ProvisioningStatus)
	}
	if lb1.OperatingStatus != "ONLINE" {
		t.Errorf("unexpected OperatingStatus: %s", lb1.OperatingStatus)
	}
	if !lb1.AdminStateUp {
		t.Error("expected AdminStateUp to be true")
	}

	// Verify second LB (creating)
	lb2 := lbs[1]
	if lb2.ID != "a2c7e6d4-8f5b-4e9a-8c2f-7b1e3d5a9c28" {
		t.Errorf("unexpected ID: %s", lb2.ID)
	}
	if lb2.Name != "backend-lb" {
		t.Errorf("unexpected Name: %s", lb2.Name)
	}
	if lb2.VipAddress != "10.20.2.100" {
		t.Errorf("unexpected VipAddress: %s", lb2.VipAddress)
	}
	if lb2.ProvisioningStatus != "PENDING_CREATE" {
		t.Errorf("unexpected ProvisioningStatus: %s", lb2.ProvisioningStatus)
	}
	if lb2.OperatingStatus != "OFFLINE" {
		t.Errorf("unexpected OperatingStatus: %s", lb2.OperatingStatus)
	}
	if lb2.AdminStateUp {
		t.Error("expected AdminStateUp to be false")
	}
}

func TestListListeners(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/listeners") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(listenerListFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeLBClient(handler)
	ctx := context.Background()

	listeners, err := ListListeners(ctx, client, "f1b9d8c2-7a3e-4d1f-92c5-e81a2d3b4f76")
	if err != nil {
		t.Fatalf("ListListeners() error: %v", err)
	}
	if len(listeners) != 2 {
		t.Fatalf("expected 2 listeners, got %d", len(listeners))
	}

	l1 := listeners[0]
	if l1.ID != "d3e4f5a1-b2c3-4d5e-6f7a-8b9c0d1e2f3a" {
		t.Errorf("unexpected ID: %s", l1.ID)
	}
	if l1.Name != "https-listener" {
		t.Errorf("unexpected Name: %s", l1.Name)
	}
	if l1.Protocol != "HTTPS" {
		t.Errorf("unexpected Protocol: %s", l1.Protocol)
	}
	if l1.ProtocolPort != 443 {
		t.Errorf("unexpected ProtocolPort: %d", l1.ProtocolPort)
	}
	if !l1.AdminStateUp {
		t.Error("expected AdminStateUp to be true")
	}

	l2 := listeners[1]
	if l2.ID != "b4c5d6e1-f2a3-4b5c-6d7e-8f9a0b1c2d3e" {
		t.Errorf("unexpected ID: %s", l2.ID)
	}
	if l2.Name != "http-listener" {
		t.Errorf("unexpected Name: %s", l2.Name)
	}
	if l2.Protocol != "HTTP" {
		t.Errorf("unexpected Protocol: %s", l2.Protocol)
	}
	if l2.ProtocolPort != 80 {
		t.Errorf("unexpected ProtocolPort: %d", l2.ProtocolPort)
	}
}
