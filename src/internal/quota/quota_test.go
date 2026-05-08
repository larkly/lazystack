package quota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

func fakeNovaClient(handler http.Handler) *gophercloud.ServiceClient {
	srv := httptest.NewServer(handler)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			HTTPClient: *srv.Client(),
		},
		Endpoint: srv.URL + "/",
	}
}

func fakeNeutronClient(handler http.Handler) *gophercloud.ServiceClient {
	srv := httptest.NewServer(handler)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			HTTPClient: *srv.Client(),
		},
		Endpoint: srv.URL + "/",
	}
}

func fakeCinderClient(handler http.Handler) *gophercloud.ServiceClient {
	srv := httptest.NewServer(handler)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			HTTPClient: *srv.Client(),
		},
		Endpoint: srv.URL + "/",
	}
}

// computeQuotasFixture is a minimal Nova os-quota-sets/detail response.
const computeQuotasFixture = `{
  "quota_set": {
    "id": "project-uuid",
    "instances": {
      "in_use": 5,
      "limit": 50,
      "reserved": 1
    },
    "cores": {
      "in_use": 10,
      "limit": 200,
      "reserved": 2
    },
    "ram": {
      "in_use": 16384,
      "limit": 524288,
      "reserved": 0
    },
    "key_pairs": {
      "in_use": 2,
      "limit": 100,
      "reserved": 0
    },
    "server_groups": {
      "in_use": 1,
      "limit": 10,
      "reserved": 0
    },
    "server_group_members": {
      "in_use": 3,
      "limit": 10,
      "reserved": 0
    },
    "floating_ips": {
      "in_use": 0,
      "limit": 10,
      "reserved": 0
    },
    "security_groups": {
      "in_use": 0,
      "limit": 10,
      "reserved": 0
    },
    "metadata_items": {
      "in_use": 0,
      "limit": 128,
      "reserved": 0
    }
  }
}`

// networkQuotasFixture is a minimal Neutron quotas/detail response.
const networkQuotasFixture = `{
  "quota": {
    "floatingip": {
      "used": 2,
      "limit": 10,
      "reserved": 0
    },
    "network": {
      "used": 3,
      "limit": 20,
      "reserved": 0
    },
    "port": {
      "used": 8,
      "limit": 500,
      "reserved": 2
    },
    "router": {
      "used": 1,
      "limit": 10,
      "reserved": 0
    },
    "security_group": {
      "used": 5,
      "limit": 50,
      "reserved": 0
    },
    "subnet": {
      "used": 4,
      "limit": 30,
      "reserved": 0
    },
    "subnetpool": {
      "used": 0,
      "limit": 10,
      "reserved": 0
    },
    "rbac_policy": {
      "used": 0,
      "limit": 25,
      "reserved": 0
    }
  }
}`

// volumeQuotasFixture is a minimal Cinder os-quota-sets/usage response.
const volumeQuotasFixture = `{
  "quota_set": {
    "id": "project-uuid",
    "volumes": {
      "in_use": 3,
      "limit": 100,
      "reserved": 0,
      "allocated": 0
    },
    "snapshots": {
      "in_use": 2,
      "limit": 200,
      "reserved": 0,
      "allocated": 0
    },
    "gigabytes": {
      "in_use": 50,
      "limit": 1000,
      "reserved": 0,
      "allocated": 0
    },
    "backups": {
      "in_use": 1,
      "limit": 50,
      "reserved": 0,
      "allocated": 0
    },
    "per_volume_gigabytes": {
      "in_use": 0,
      "limit": 1000,
      "reserved": 0,
      "allocated": 0
    }
  }
}`

func TestGetComputeQuotas(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/os-quota-sets/") && strings.Contains(r.URL.Path, "/detail") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(computeQuotasFixture))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	client := fakeNovaClient(handler)
	ctx := context.Background()

	quotas, err := GetComputeQuotas(ctx, client, "project-uuid")
	if err != nil {
		t.Fatalf("GetComputeQuotas() error: %v", err)
	}
	if len(quotas) != 5 {
		t.Fatalf("expected 5 quota resources, got %d", len(quotas))
	}

	// Verify Instances
	if quotas[0].Resource != "Instances" {
		t.Errorf("unexpected resource: %s", quotas[0].Resource)
	}
	if quotas[0].Used != 5 {
		t.Errorf("Instances Used: expected 5, got %d", quotas[0].Used)
	}
	if quotas[0].Limit != 50 {
		t.Errorf("Instances Limit: expected 50, got %d", quotas[0].Limit)
	}

	// Verify Cores
	if quotas[1].Resource != "Cores" {
		t.Errorf("unexpected resource: %s", quotas[1].Resource)
	}
	if quotas[1].Used != 10 {
		t.Errorf("Cores Used: expected 10, got %d", quotas[1].Used)
	}
	if quotas[1].Limit != 200 {
		t.Errorf("Cores Limit: expected 200, got %d", quotas[1].Limit)
	}

	// Verify RAM
	if quotas[2].Resource != "RAM (MB)" {
		t.Errorf("unexpected resource: %s", quotas[2].Resource)
	}
	if quotas[2].Used != 16384 {
		t.Errorf("RAM Used: expected 16384, got %d", quotas[2].Used)
	}
	if quotas[2].Limit != 524288 {
		t.Errorf("RAM Limit: expected 524288, got %d", quotas[2].Limit)
	}

	// Verify Key Pairs
	if quotas[3].Resource != "Key Pairs" {
		t.Errorf("unexpected resource: %s", quotas[3].Resource)
	}
	if quotas[3].Used != 2 {
		t.Errorf("Key Pairs Used: expected 2, got %d", quotas[3].Used)
	}
	if quotas[3].Limit != 100 {
		t.Errorf("Key Pairs Limit: expected 100, got %d", quotas[3].Limit)
	}

	// Verify Server Groups
	if quotas[4].Resource != "Server Groups" {
		t.Errorf("unexpected resource: %s", quotas[4].Resource)
	}
	if quotas[4].Used != 1 {
		t.Errorf("Server Groups Used: expected 1, got %d", quotas[4].Used)
	}
	if quotas[4].Limit != 10 {
		t.Errorf("Server Groups Limit: expected 10, got %d", quotas[4].Limit)
	}
}

func TestGetNetworkQuotas(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/quotas/") && strings.Contains(r.URL.Path, "/detail") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(networkQuotasFixture))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	client := fakeNeutronClient(handler)
	ctx := context.Background()

	quotas, err := GetNetworkQuotas(ctx, client, "project-uuid")
	if err != nil {
		t.Fatalf("GetNetworkQuotas() error: %v", err)
	}
	if len(quotas) != 6 {
		t.Fatalf("expected 6 quota resources, got %d", len(quotas))
	}

	// Verify Floating IPs
	if quotas[0].Resource != "Floating IPs" {
		t.Errorf("unexpected resource: %s", quotas[0].Resource)
	}
	if quotas[0].Used != 2 {
		t.Errorf("Floating IPs Used: expected 2, got %d", quotas[0].Used)
	}
	if quotas[0].Limit != 10 {
		t.Errorf("Floating IPs Limit: expected 10, got %d", quotas[0].Limit)
	}

	// Verify Networks
	if quotas[1].Resource != "Networks" {
		t.Errorf("unexpected resource: %s", quotas[1].Resource)
	}
	if quotas[1].Used != 3 {
		t.Errorf("Networks Used: expected 3, got %d", quotas[1].Used)
	}
	if quotas[1].Limit != 20 {
		t.Errorf("Networks Limit: expected 20, got %d", quotas[1].Limit)
	}

	// Verify Ports
	if quotas[2].Resource != "Ports" {
		t.Errorf("unexpected resource: %s", quotas[2].Resource)
	}
	if quotas[2].Used != 8 {
		t.Errorf("Ports Used: expected 8, got %d", quotas[2].Used)
	}
	if quotas[2].Limit != 500 {
		t.Errorf("Ports Limit: expected 500, got %d", quotas[2].Limit)
	}

	// Verify Routers
	if quotas[3].Resource != "Routers" {
		t.Errorf("unexpected resource: %s", quotas[3].Resource)
	}
	if quotas[3].Used != 1 {
		t.Errorf("Routers Used: expected 1, got %d", quotas[3].Used)
	}
	if quotas[3].Limit != 10 {
		t.Errorf("Routers Limit: expected 10, got %d", quotas[3].Limit)
	}

	// Verify Security Groups
	if quotas[4].Resource != "Security Groups" {
		t.Errorf("unexpected resource: %s", quotas[4].Resource)
	}
	if quotas[4].Used != 5 {
		t.Errorf("Security Groups Used: expected 5, got %d", quotas[4].Used)
	}
	if quotas[4].Limit != 50 {
		t.Errorf("Security Groups Limit: expected 50, got %d", quotas[4].Limit)
	}

	// Verify Subnets
	if quotas[5].Resource != "Subnets" {
		t.Errorf("unexpected resource: %s", quotas[5].Resource)
	}
	if quotas[5].Used != 4 {
		t.Errorf("Subnets Used: expected 4, got %d", quotas[5].Used)
	}
	if quotas[5].Limit != 30 {
		t.Errorf("Subnets Limit: expected 30, got %d", quotas[5].Limit)
	}
}

func TestGetVolumeQuotas(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/os-quota-sets/") && r.URL.Query().Get("usage") == "true" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(volumeQuotasFixture))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	client := fakeCinderClient(handler)
	ctx := context.Background()

	quotas, err := GetVolumeQuotas(ctx, client, "project-uuid")
	if err != nil {
		t.Fatalf("GetVolumeQuotas() error: %v", err)
	}
	if len(quotas) != 4 {
		t.Fatalf("expected 4 quota resources, got %d", len(quotas))
	}

	// Verify Volumes
	if quotas[0].Resource != "Volumes" {
		t.Errorf("unexpected resource: %s", quotas[0].Resource)
	}
	if quotas[0].Used != 3 {
		t.Errorf("Volumes Used: expected 3, got %d", quotas[0].Used)
	}
	if quotas[0].Limit != 100 {
		t.Errorf("Volumes Limit: expected 100, got %d", quotas[0].Limit)
	}

	// Verify Gigabytes
	if quotas[1].Resource != "Gigabytes" {
		t.Errorf("unexpected resource: %s", quotas[1].Resource)
	}
	if quotas[1].Used != 50 {
		t.Errorf("Gigabytes Used: expected 50, got %d", quotas[1].Used)
	}
	if quotas[1].Limit != 1000 {
		t.Errorf("Gigabytes Limit: expected 1000, got %d", quotas[1].Limit)
	}

	// Verify Snapshots
	if quotas[2].Resource != "Snapshots" {
		t.Errorf("unexpected resource: %s", quotas[2].Resource)
	}
	if quotas[2].Used != 2 {
		t.Errorf("Snapshots Used: expected 2, got %d", quotas[2].Used)
	}
	if quotas[2].Limit != 200 {
		t.Errorf("Snapshots Limit: expected 200, got %d", quotas[2].Limit)
	}

	// Verify Backups
	if quotas[3].Resource != "Backups" {
		t.Errorf("unexpected resource: %s", quotas[3].Resource)
	}
	if quotas[3].Used != 1 {
		t.Errorf("Backups Used: expected 1, got %d", quotas[3].Used)
	}
	if quotas[3].Limit != 50 {
		t.Errorf("Backups Limit: expected 50, got %d", quotas[3].Limit)
	}
}

func TestGetComputeQuotas_InvalidProjectID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	client := fakeNovaClient(handler)
	ctx := context.Background()

	_, err := GetComputeQuotas(ctx, client, "")
	if err == nil {
		t.Fatal("expected error for empty projectID, got nil")
	}
	if !strings.Contains(err.Error(), "projectID is required") {
		t.Errorf("expected error to contain 'projectID is required', got: %v", err)
	}
}

func TestGetNetworkQuotas_InvalidProjectID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	client := fakeNeutronClient(handler)
	ctx := context.Background()

	_, err := GetNetworkQuotas(ctx, client, "")
	if err == nil {
		t.Fatal("expected error for empty projectID, got nil")
	}
	if !strings.Contains(err.Error(), "projectID is required") {
		t.Errorf("expected error to contain 'projectID is required', got: %v", err)
	}
}

func TestGetVolumeQuotas_InvalidProjectID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	client := fakeCinderClient(handler)
	ctx := context.Background()

	_, err := GetVolumeQuotas(ctx, client, "")
	if err == nil {
		t.Fatal("expected error for empty projectID, got nil")
	}
	if !strings.Contains(err.Error(), "projectID is required") {
		t.Errorf("expected error to contain 'projectID is required', got: %v", err)
	}
}
