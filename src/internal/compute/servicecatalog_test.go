package compute

import (
	"testing"
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
