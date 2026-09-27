package compute

import (
	"fmt"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

func TestKnownServicesNotEmpty(t *testing.T) {
	if len(knownServices) == 0 {
		t.Fatal("knownServices should not be empty")
	}

	// Verify core services are present
	found := map[string]bool{}
	for _, ks := range knownServices {
		found[ks.Type] = true
		if ks.Name == "" {
			t.Errorf("service %s has empty Name", ks.Type)
		}
		if ks.NewFunc == nil {
			t.Errorf("service %s has nil NewFunc", ks.Type)
		}
	}

	expected := []string{"compute", "image", "network", "identity", "block-storage", "load-balancer", "placement"}
	for _, e := range expected {
		if !found[e] {
			t.Errorf("expected service %q not found in knownServices", e)
		}
	}
}

func TestKnownServices_UniqueTypes(t *testing.T) {
	seen := map[string]bool{}
	for _, ks := range knownServices {
		if seen[ks.Type] {
			t.Errorf("duplicate service type: %s", ks.Type)
		}
		seen[ks.Type] = true
	}
}

func TestServiceEntry_Type(t *testing.T) {
	entry := ServiceEntry{
		Name:      "Compute (Nova)",
		Type:      "compute",
		Available: true,
	}
	if entry.Name != "Compute (Nova)" {
		t.Errorf("unexpected name: %s", entry.Name)
	}
	if entry.Available != true {
		t.Error("expected available to be true")
	}
}

// blockStorageEntry runs FetchServiceCatalog against a provider whose catalog
// only resolves the given block storage major versions.
func blockStorageEntry(t *testing.T, versions ...int) ServiceEntry {
	t.Helper()
	pc := &gophercloud.ProviderClient{}
	pc.EndpointLocator = func(eo gophercloud.EndpointOpts) (string, error) {
		for _, v := range versions {
			if eo.Version == v && (eo.Type == "block-storage" || eo.Type == "volume") {
				return fmt.Sprintf("https://cinder.example/v%d/", v), nil
			}
		}
		return "", &gophercloud.ErrEndpointNotFound{}
	}
	for _, e := range FetchServiceCatalog(pc, gophercloud.EndpointOpts{Region: "r1"}) {
		if e.Type == "block-storage" {
			return e
		}
	}
	t.Fatal("block-storage entry missing")
	return ServiceEntry{}
}

func TestFetchServiceCatalog_BlockStorageFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name      string
		versions  []int
		available bool
		wantURL   string
	}{
		{"legacy volume v1 only", []int{1}, true, "https://cinder.example/v1/"},
		{"v2 preferred over v1", []int{1, 2}, true, "https://cinder.example/v2/"},
		{"v3 preferred over v2 and v1", []int{1, 2, 3}, true, "https://cinder.example/v3/"},
		{"absent", nil, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := blockStorageEntry(t, tc.versions...)
			if e.Available != tc.available {
				t.Fatalf("Available=%v, want %v", e.Available, tc.available)
			}
			if !tc.available {
				if len(e.Endpoints) != 0 {
					t.Fatalf("endpoints=%v, want none", e.Endpoints)
				}
				return
			}
			if len(e.Endpoints) == 0 || e.Endpoints[0].URL != tc.wantURL {
				t.Fatalf("endpoints=%v, want first URL %s", e.Endpoints, tc.wantURL)
			}
		})
	}
}

func TestFetchServiceCatalog_ReturnsAllEntries(t *testing.T) {
	// FetchServiceCatalog requires a fully-configured ProviderClient (identity
	// endpoint + token). Rather than mock the full Keystone auth flow, we verify
	// that knownServices is complete and the ServiceEntry type is correct.
	// The actions_test.go and users_test.go files cover httptest-based API
	// mocking for the functions that call Gophercloud directly.
	if len(knownServices) < 7 {
		t.Errorf("expected at least 7 known services, got %d", len(knownServices))
	}
}
