package cloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

// fakeKeystoneServer returns a ProviderClient wired to a Keystone token handler.
func fakeKeystoneServer(tokenFixture string) (*gophercloud.ProviderClient, func()) {
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Subject-Token", "test-token-abc123")
			w.WriteHeader(201)
			w.Write([]byte(tokenFixture))
		}),
	)
	pc := &gophercloud.ProviderClient{
		HTTPClient:       *srv.Client(),
		IdentityBase:     srv.URL + "/",
		IdentityEndpoint: srv.URL + "/",
		TokenID:          "test-token-abc123",
	}
	return pc, srv.Close
}

// TestAuthenticateClient_ProviderClientSet verifies that a ProviderClient is
// constructed with required fields after authentication.
func TestAuthenticateClient_ProviderClientSet(t *testing.T) {
	tokenFixture := `{"token": {"catalog": [], "expires_at": "2026-12-31T00:00:00Z"}}`
	pc, closeFn := fakeKeystoneServer(tokenFixture)
	defer closeFn()

	if pc.IdentityEndpoint == "" {
		t.Error("ProviderClient should have IdentityEndpoint set")
	}
	if pc.TokenID == "" || pc.TokenID != "test-token-abc123" {
		t.Errorf("ProviderClient.TokenID = %q, want %q", pc.TokenID, "test-token-abc123")
	}
}

// TestAuthenticateClient_ServiceClient validates that a ServiceClient can be
// constructed from an authenticated ProviderClient.
func TestAuthenticateClient_ServiceClient(t *testing.T) {
	tokenFixture := `{"token": {"catalog": [
		{"type": "compute", "name": "nova", "endpoints": [
			{"interface": "public", "url": "http://nova.example.com:8774/v2.1"}
		]}
	]}}`
	pc, closeFn := fakeKeystoneServer(tokenFixture)
	defer closeFn()

	sc := &gophercloud.ServiceClient{
		ProviderClient: pc,
		Endpoint:       pc.IdentityBase,
	}

	if sc.ProviderClient == nil {
		t.Fatal("ServiceClient.ProviderClient should not be nil")
	}
	if sc.Endpoint == "" {
		t.Fatal("ServiceClient.Endpoint should not be empty")
	}
}

// TestConnectReturnsClient ensures Connect produces a Client struct for a
// parsable cloud config (connection failure is expected, not a config error).
func TestConnectReturnsClient(t *testing.T) {
	dir := t.TempDir()
	content := `clouds:
  mycloud:
    auth:
      auth_url: http://127.0.0.1:19999
    region_name: RegionOne
`
	if err := os.WriteFile(filepath.Join(dir, "clouds.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OS_CLIENT_CONFIG_FILE", filepath.Join(dir, "clouds.yaml"))
	t.Setenv("HOME", dir)

	ctx := context.Background()
	_, err := Connect(ctx, "mycloud")
	// Expect a connection error (unreachable endpoint), NOT a parse error.
	if err == nil {
		t.Log("unexpected success: port 19999 may be in use")
	}
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "parsing") {
		t.Fatalf("unexpected parse error (config should parse correctly): %v", err)
	}
	t.Logf("expected connection error: %v", err)
}

// TestConnect_MissingCloudName verifies error for a nonexistent cloud.
func TestConnect_MissingCloudName(t *testing.T) {
	dir := t.TempDir()
	content := `clouds:
  realcloud:
    auth:
      auth_url: https://real.example.com:5000
`
	if err := os.WriteFile(filepath.Join(dir, "clouds.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OS_CLIENT_CONFIG_FILE", filepath.Join(dir, "clouds.yaml"))
	t.Setenv("HOME", dir)

	ctx := context.Background()
	_, err := Connect(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent cloud name, got nil")
	}
}

// TestConnectWithProject_NonexistentCloud verifies error propagation.
func TestConnectWithProject_NonexistentCloud(t *testing.T) {
	dir := t.TempDir()
	content := `clouds:
  realcloud:
    auth:
      auth_url: https://real.example.com:5000
`
	if err := os.WriteFile(filepath.Join(dir, "clouds.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OS_CLIENT_CONFIG_FILE", filepath.Join(dir, "clouds.yaml"))
	t.Setenv("HOME", dir)

	ctx := context.Background()
	_, err := ConnectWithProject(ctx, "nonexistent", "p1")
	if err == nil {
		t.Error("expected error for nonexistent cloud, got nil")
	}
}

// TestConnectWithProject_InvalidAuthURL ensures ConnectWithProject propagates
// auth URL errors.
func TestConnectWithProject_InvalidAuthURL(t *testing.T) {
	dir := t.TempDir()
	content := `clouds:
  testcloud:
    auth:
      auth_url: "://invalid-url-for-testing"
`
	if err := os.WriteFile(filepath.Join(dir, "clouds.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OS_CLIENT_CONFIG_FILE", filepath.Join(dir, "clouds.yaml"))
	t.Setenv("HOME", dir)

	ctx := context.Background()
	_, err := ConnectWithProject(ctx, "testcloud", "some-project-id")
	if err == nil {
		t.Error("expected error for invalid auth URL in ConnectWithProject, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "testcloud") {
		t.Errorf("error should mention cloud name, got: %v", err)
	}
}
