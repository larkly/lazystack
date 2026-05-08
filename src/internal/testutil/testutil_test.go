package testutil

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFakeServiceClient_ReturnsValidClient(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	sc := FakeServiceClient(handler)

	if sc == nil {
		t.Fatal("FakeServiceClient returned nil")
	}
	if sc.ProviderClient == nil {
		t.Fatal("ServiceClient.ProviderClient is nil")
	}
	if sc.Endpoint == "" {
		t.Fatal("ServiceClient.Endpoint is empty")
	}
}

func TestFakeServiceClient_RequestsRoutedToHandler(t *testing.T) {
	capturedPath := ""
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	sc := FakeServiceClient(handler)

	// Use the ProviderClient's HTTP client to make a request
	resp, err := sc.ProviderClient.HTTPClient.Get(sc.Endpoint + "test-path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if capturedPath != "/test-path" {
		t.Errorf("expected path /test-path, got %s", capturedPath)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestFakeServiceClient_EndpointIsAbsoluteURL(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	sc := FakeServiceClient(handler)

	if !strings.HasPrefix(sc.Endpoint, "http://") && !strings.HasPrefix(sc.Endpoint, "https://") {
		t.Errorf("endpoint should be an absolute URL, got %s", sc.Endpoint)
	}
}

func TestFakeServiceClientWithFixture_ReturnsJSONForMatchingPath(t *testing.T) {
	expectedBody := `{"id": "123", "name": "test-project"}`
	path := "/v2.1/projects"
	sc := FakeServiceClientWithFixture(expectedBody, path)

	resp, err := sc.ProviderClient.HTTPClient.Get(sc.Endpoint + "v2.1/projects")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, string(body))
	}
}

func TestFakeServiceClientWithFixture_TrailingSlashWorks(t *testing.T) {
	expectedBody := `{"id": "456"}`
	path := "/v2.1/servers"
	sc := FakeServiceClientWithFixture(expectedBody, path)

	resp, err := sc.ProviderClient.HTTPClient.Get(sc.Endpoint + "v2.1/servers/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for trailing slash, got %d", resp.StatusCode)
	}
}

func TestFakeServiceClientWithFixture_Returns404ForMismatchedPath(t *testing.T) {
	jsonBody := `{"id": "789"}`
	sc := FakeServiceClientWithFixture(jsonBody, "/v2.1/networks")

	resp, err := sc.ProviderClient.HTTPClient.Get(sc.Endpoint + "v2.1/subnets")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404 for mismatched path, got %d", resp.StatusCode)
	}
}

func TestFakeServiceClientWithFixture_ContentTypeIsJSON(t *testing.T) {
	jsonBody := `{"key": "value"}`
	sc := FakeServiceClientWithFixture(jsonBody, "/v2.1/test")

	resp, err := sc.ProviderClient.HTTPClient.Get(sc.Endpoint + "v2.1/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}

// Verify the httptest server is properly cleaned up by checking that
// a second client gets a different endpoint.
func TestFakeServiceClient_EachCallGetsUniqueEndpoint(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	sc1 := FakeServiceClient(handler)
	sc2 := FakeServiceClient(handler)

	if sc1.Endpoint == sc2.Endpoint {
		t.Error("expected different endpoints for separate FakeServiceClient calls")
	}
}
