package loadbalancer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func fakeLBClient(t *testing.T, handler http.Handler) *gophercloud.ServiceClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
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

	client := fakeLBClient(t, handler)
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

	client := fakeLBClient(t, handler)
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

// lbGetFixture renders a single-LB GET response with the given provisioning
// status, as returned by loadbalancers.Get.
func lbGetFixture(status string) string {
	return `{"loadbalancer": {"id": "lb-123", "name": "test-lb", "provisioning_status": "` + status + `", "operating_status": "ONLINE"}}`
}

// lbResponse is one scripted HTTP response for the polling tests: either an
// error status code or a 200 with a JSON body. A zero code means 200.
type lbResponse struct {
	code int
	body string
}

// scriptHandler serves responses in order, repeating the last one once the
// script is exhausted, and records the number of handled requests.
func scriptHandler(responses []lbResponse, calls *int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*calls++
		i := *calls - 1
		if i >= len(responses) {
			i = len(responses) - 1
		}
		resp := responses[i]
		if resp.code != http.StatusOK && resp.code != 0 {
			w.WriteHeader(resp.code)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp.body))
	}
}

func TestWaitForActive(t *testing.T) {
	tests := []struct {
		name       string
		responses  []lbResponse
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "returns success once status ACTIVE",
			responses: []lbResponse{
				{body: lbGetFixture("PENDING_CREATE")},
				{body: lbGetFixture("ACTIVE")},
			},
		},
		{
			name: "retries past transient 500s then succeeds",
			responses: []lbResponse{
				{code: http.StatusInternalServerError},
				{code: http.StatusInternalServerError},
				{body: lbGetFixture("ACTIVE")},
			},
		},
		{
			name:       "aborts immediately on 404",
			responses:  []lbResponse{{code: http.StatusNotFound}},
			wantErr:    true,
			wantErrMsg: "load balancer lb-123 not found",
		},
		{
			name:       "aborts on ERROR provisioning status",
			responses:  []lbResponse{{body: lbGetFixture("ERROR_DELETING")}},
			wantErr:    true,
			wantErrMsg: "LB lb-123 entered ERROR_DELETING",
		},
		{
			name: "auth errors abort immediately",
			responses: []lbResponse{
				{code: http.StatusForbidden},
				{body: lbGetFixture("ACTIVE")},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls int
			client := fakeLBClient(t, scriptHandler(tt.responses, &calls))
			ctx := context.Background()

			err := WaitForActive(ctx, client, "lb-123", 30*time.Second)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("WaitForActive() error = nil, want %q", tt.wantErrMsg)
				}
				if tt.wantErrMsg != "" && err.Error() != tt.wantErrMsg {
					t.Errorf("WaitForActive() error = %q, want %q", err.Error(), tt.wantErrMsg)
				}
				if calls != 1 {
					t.Errorf("expected immediate abort after 1 poll, got %d", calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("WaitForActive() error: %v", err)
			}
		})
	}
}

func TestWaitForActiveNilClient(t *testing.T) {
	err := WaitForActive(context.Background(), nil, "lb-123", time.Second)
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}
	want := "load balancer (Octavia) service is not available in this cloud"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestListLoadBalancersNilClient(t *testing.T) {
	_, err := ListLoadBalancers(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}
	want := "load balancer (Octavia) service is not available in this cloud"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}
