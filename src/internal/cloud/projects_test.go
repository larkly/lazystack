package cloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

// fakeIdentityClient creates a test ProviderClient wired to a Keystone API handler.
func fakeIdentityClient(t testing.TB, handler http.Handler) *gophercloud.ProviderClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	pc := &gophercloud.ProviderClient{
		HTTPClient:       *srv.Client(),
		IdentityBase:     srv.URL + "/",
		IdentityEndpoint: srv.URL + "/",
		TokenID:          "test-token",
	}
	pc.EndpointLocator = func(eo gophercloud.EndpointOpts) (string, error) {
		return srv.URL, nil
	}
	return pc
}

// TestListAccessibleProjects_Empty verifies an empty project list returns zero results.
func TestListAccessibleProjects_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "auth/projects") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"projects": []}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	pc := fakeIdentityClient(t, handler)
	eo := gophercloud.EndpointOpts{}
	ctx := context.Background()

	projects, err := ListAccessibleProjects(ctx, pc, eo)
	if err != nil {
		t.Fatalf("ListAccessibleProjects() error: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

// TestListAccessibleProjects_AllDisabled ensures disabled projects are filtered out.
func TestListAccessibleProjects_AllDisabled(t *testing.T) {
	projectsJSON := `{
  "projects": [
    {"id": "p1", "name": "disabled1", "enabled": false},
    {"id": "p2", "name": "disabled2", "enabled": false},
    {"id": "p3", "name": "disabled3", "enabled": false}
  ]
}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "auth/projects") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(projectsJSON))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	pc := fakeIdentityClient(t, handler)
	eo := gophercloud.EndpointOpts{}
	ctx := context.Background()

	projects, err := ListAccessibleProjects(ctx, pc, eo)
	if err != nil {
		t.Fatalf("ListAccessibleProjects() error: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected 0 enabled projects (all disabled), got %d", len(projects))
	}
}

// TestListAccessibleProjects_IdentityError verifies error propagation when
// the identity endpoint is unreachable.
func TestListAccessibleProjects_IdentityError(t *testing.T) {
	pc := &gophercloud.ProviderClient{
		IdentityEndpoint: "http://127.0.0.1:19999",
	}
	eo := gophercloud.EndpointOpts{}
	ctx := context.Background()

	_, err := ListAccessibleProjects(ctx, pc, eo)
	if err == nil {
		t.Error("expected error for unreachable identity endpoint, got nil")
	}
}

// Fake clients must shut their test server down when the test that created
// them finishes, so repeated tests do not accumulate listeners.
func TestFakeClientsCloseWithTest(t *testing.T) {
	var endpoints []string
	t.Run("use", func(t *testing.T) {
		endpoints = append(endpoints, fakeIdentityClient(t, http.NotFoundHandler()).IdentityEndpoint)
		endpoints = append(endpoints, fakeProviderClient(t, http.NotFoundHandler()).IdentityEndpoint)
	})
	for _, ep := range endpoints {
		if resp, err := http.Get(ep); err == nil {
			resp.Body.Close()
			t.Errorf("test server %s still running after its test finished", ep)
		}
	}
}
