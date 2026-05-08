package compute

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

// usersFixture is a minimal Keystone v3 API paginated user list response.
const usersFixture = `{
  "users": [
    {
      "id": "user-001",
      "name": "alice",
      "enabled": true,
      "description": "Primary admin",
      "domain_id": "default",
      "email": "alice@example.com"
    },
    {
      "id": "user-002",
      "name": "bob",
      "enabled": false,
      "description": "Disabled user",
      "domain_id": "default",
      "email": "bob@example.com"
    },
    {
      "id": "user-003",
      "name": "carol",
      "enabled": true,
      "description": "",
      "domain_id": "dev-domain"
    }
  ]
}`

// fakeIdentityProviderClient creates a ProviderClient that routes all
// Keystone API calls to the given httptest handler. Sets IdentityEndpoint
// and EndpointLocator to avoid nil pointer dereferences in NewIdentityV3.
func fakeIdentityProviderClient(handler http.Handler) (*gophercloud.ProviderClient, func()) {
	srv := httptest.NewServer(handler)
	pc := &gophercloud.ProviderClient{
		HTTPClient:       *srv.Client(),
		IdentityEndpoint: srv.URL + "/",
		EndpointLocator: func(eo gophercloud.EndpointOpts) (string, error) {
			return srv.URL + "/", nil
		},
	}
	return pc, srv.Close
}

func TestListUsers(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(usersFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	pc, cleanup := fakeIdentityProviderClient(handler)
	defer cleanup()

	eo := gophercloud.EndpointOpts{
		Availability: gophercloud.AvailabilityPublic,
		Region:       "RegionOne",
	}
	ctx := context.Background()

	users, err := ListUsers(ctx, pc, eo)
	if err != nil {
		t.Fatalf("ListUsers() error: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("expected 3 users, got %d", len(users))
	}

	// Verify first user
	u1 := users[0]
	if u1.ID != "user-001" {
		t.Errorf("unexpected ID: %s", u1.ID)
	}
	if u1.Name != "alice" {
		t.Errorf("unexpected Name: %s", u1.Name)
	}
	if !u1.Enabled {
		t.Error("expected alice to be enabled")
	}
	if u1.DomainID != "default" {
		t.Errorf("unexpected DomainID: %s", u1.DomainID)
	}

	// Verify second user (disabled)
	u2 := users[1]
	if u2.Enabled {
		t.Error("expected bob to be disabled")
	}

	// Verify third user (no description)
	u3 := users[2]
	if u3.Description != "" {
		t.Errorf("expected empty description, got %q", u3.Description)
	}
}

func TestListUsers_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"users": []}`))
	})

	pc, cleanup := fakeIdentityProviderClient(handler)
	defer cleanup()

	eo := gophercloud.EndpointOpts{
		Availability: gophercloud.AvailabilityPublic,
		Region:       "RegionOne",
	}
	ctx := context.Background()
	users, err := ListUsers(ctx, pc, eo)
	if err != nil {
		t.Fatalf("ListUsers() error: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestSetUserEnabled(t *testing.T) {
	var method, path, body string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		body = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"user": {"id": "user-001", "name": "alice", "enabled": false}}`))
	})

	pc, cleanup := fakeIdentityProviderClient(handler)
	defer cleanup()

	eo := gophercloud.EndpointOpts{
		Availability: gophercloud.AvailabilityPublic,
		Region:       "RegionOne",
	}
	ctx := context.Background()
	err := SetUserEnabled(ctx, pc, eo, "user-001", false)
	if err != nil {
		t.Fatalf("SetUserEnabled() error: %v", err)
	}

	if method != http.MethodPatch {
		t.Errorf("expected PATCH, got %s", method)
	}
	if !strings.Contains(path, "user-001") {
		t.Errorf("path %s should contain user-001", path)
	}
	if !strings.Contains(body, "enabled") {
		t.Errorf("body %s should contain 'enabled'", body)
	}
}

func TestDeleteUser(t *testing.T) {
	var method, path string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})

	pc, cleanup := fakeIdentityProviderClient(handler)
	defer cleanup()

	eo := gophercloud.EndpointOpts{
		Availability: gophercloud.AvailabilityPublic,
		Region:       "RegionOne",
	}
	ctx := context.Background()
	err := DeleteUser(ctx, pc, eo, "user-003")
	if err != nil {
		t.Fatalf("DeleteUser() error: %v", err)
	}

	if method != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", method)
	}
	if !strings.Contains(path, "user-003") {
		t.Errorf("path %s should contain user-003", path)
	}
}
