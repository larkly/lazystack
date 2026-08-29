package cloud

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// ---------------------------------------------------------------------------
// Tests for Nova microversion negotiation (#193, #194)
// ---------------------------------------------------------------------------

// fakeComputeClient wires a compute ServiceClient to an httptest server so
// negotiateNovaMicroversion can be exercised end-to-end.
func fakeComputeClient(handler http.Handler) (*gophercloud.ServiceClient, func()) {
	srv := httptest.NewServer(handler)
	compute := &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{HTTPClient: *srv.Client()},
		Endpoint:       srv.URL + "/",
		Type:           "compute",
	}
	return compute, srv.Close
}

func TestNegotiateNovaMicroversion_SingularVersionDoc(t *testing.T) {
	compute, closeFn := fakeComputeClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version": {"id": "v2.1", "version": "2.88", "min_version": "2.1"}}`))
	}))
	defer closeFn()

	max, used, warn := negotiateNovaMicroversion(context.Background(), compute)
	if max != "2.88" || used != "2.88" {
		t.Errorf("got max=%q used=%q, want 2.88/2.88", max, used)
	}
	if warn == "" {
		t.Error("expected non-empty degradation warning below the 2.100 ceiling")
	}
}

func TestNegotiateNovaMicroversion_VersionsArray(t *testing.T) {
	compute, closeFn := fakeComputeClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"versions": [
			{"id": "v2.0", "version": "", "min_version": ""},
			{"id": "v2.1", "version": "2.95", "min_version": "2.1"}
		]}`))
	}))
	defer closeFn()

	max, used, warn := negotiateNovaMicroversion(context.Background(), compute)
	if max != "2.95" || used != "2.95" {
		t.Errorf("got max=%q used=%q, want 2.95/2.95", max, used)
	}
	if warn == "" {
		t.Error("expected non-empty degradation warning below the 2.100 ceiling")
	}
}

func TestNegotiateNovaMicroversion_JunkBody(t *testing.T) {
	compute, closeFn := fakeComputeClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>not json</html>"))
	}))
	defer closeFn()

	max, used, warn := negotiateNovaMicroversion(context.Background(), compute)
	if max != "unknown" || used != "2.100" || warn != "" {
		t.Errorf("got (%q, %q, %q), want (unknown, 2.100, \"\")", max, used, warn)
	}
}

func TestNegotiateNovaMicroversion_ServerError(t *testing.T) {
	compute, closeFn := fakeComputeClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer closeFn()

	max, used, warn := negotiateNovaMicroversion(context.Background(), compute)
	if max != "unknown" || used != "2.100" || warn != "" {
		t.Errorf("got (%q, %q, %q), want (unknown, 2.100, \"\")", max, used, warn)
	}
}

func TestNegotiateNovaMicroversion_AboveCeiling(t *testing.T) {
	compute, closeFn := fakeComputeClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version": {"id": "v2.1", "version": "2.150", "min_version": "2.1"}}`))
	}))
	defer closeFn()

	max, used, warn := negotiateNovaMicroversion(context.Background(), compute)
	if max != "2.150" {
		t.Errorf("max = %q, want 2.150", max)
	}
	if used != "2.100" {
		t.Errorf("used = %q, want capped 2.100", used)
	}
	if warn != "" {
		t.Errorf("warning = %q, want empty at ceiling", warn)
	}
}

func TestNegotiateNovaMicroversion_NoMicroversionHeader(t *testing.T) {
	var sawMicroversionHeader, wrongPath bool
	compute, closeFn := fakeComputeClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			wrongPath = true
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("X-OpenStack-Nova-API-Version") != "" {
			sawMicroversionHeader = true
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version": {"id": "v2.1", "version": "2.88", "min_version": "2.1"}}`))
	}))
	defer closeFn()

	_, used, _ := negotiateNovaMicroversion(context.Background(), compute)
	if sawMicroversionHeader {
		t.Error("discovery request must not carry X-OpenStack-Nova-API-Version")
	}
	if wrongPath {
		t.Error("discovery must hit the server root path /")
	}
	if used != "2.88" {
		t.Errorf("used = %q, want 2.88 (discovery request must have succeeded)", used)
	}
}

func TestNegotiateNovaMicroversion_MultipleChoices(t *testing.T) {
	compute, closeFn := fakeComputeClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMultipleChoices)
		w.Write([]byte(`{"versions": [
			{"id": "v2.0", "version": "", "min_version": ""},
			{"id": "v2.1", "version": "2.95", "min_version": "2.1"}
		]}`))
	}))
	defer closeFn()

	max, used, warn := negotiateNovaMicroversion(context.Background(), compute)
	if max != "2.95" || used != "2.95" {
		t.Errorf("got max=%q used=%q, want 2.95/2.95 for a 300 response", max, used)
	}
	if warn == "" {
		t.Error("expected non-empty degradation warning below the 2.100 ceiling")
	}
}

func TestMinMicroversion(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"2.88", "2.100", "2.88"},
		{"2.150", "2.100", "2.100"},
		{"2.1", "2.1", "2.1"},
		{"3.0", "2.100", "2.100"},
		{"2.9", "2.10", "2.9"},
	}
	for _, c := range cases {
		if got := minMicroversion(c.a, c.b); got != c.want {
			t.Errorf("minMicroversion(%q, %q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}

func TestParseMicroversion(t *testing.T) {
	valid := map[string][2]int{
		"2.1":   {2, 1},
		"2.100": {2, 100},
		"2.1.5": {2, 1},
	}
	for v, want := range valid {
		major, minor, err := parseMicroversion(v)
		if err != nil || major != want[0] || minor != want[1] {
			t.Errorf("parseMicroversion(%q) = (%d, %d, %v), want (%d, %d, nil)", v, major, minor, err, want[0], want[1])
		}
	}
	invalid := []string{"", "2", "a.b", "2.x", ".5", "v2.1"}
	for _, v := range invalid {
		if _, _, err := parseMicroversion(v); err == nil {
			t.Errorf("parseMicroversion(%q) = nil error, want error", v)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests for the retry transport (#197)
// ---------------------------------------------------------------------------

func retryTestClient() *http.Client {
	return &http.Client{Transport: &retryTransport{Base: http.DefaultTransport}}
}

func TestRetryTransport_Retries429(t *testing.T) {
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := retryTestClient().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 after retries", resp.StatusCode)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestRetryTransport_BodyReplay(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	// A plain io.Reader wrapper: http.NewRequest leaves GetBody nil, forcing
	// the transport to buffer and replay the body itself. PUT is used because
	// it is idempotent and thus retryable on 503 (POST is not).
	req, err := http.NewRequest(http.MethodPut, srv.URL, struct{ io.Reader }{strings.NewReader("hello")})
	if err != nil {
		t.Fatal(err)
	}
	if req.GetBody != nil {
		t.Fatal("test setup: expected req.GetBody to be nil")
	}

	resp, err := retryTestClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 after exhausting retries", resp.StatusCode)
	}
	if got := len(bodies); got != 4 {
		t.Errorf("attempts = %d, want 4 (1 initial + 3 retries)", got)
	}
	for i, b := range bodies {
		if b != "hello" {
			t.Errorf("attempt %d body = %q, want %q", i, b, "hello")
		}
	}
	// The request must remain usable after RoundTrip returns.
	rest, err := io.ReadAll(req.Body)
	if err != nil || string(rest) != "hello" {
		t.Errorf("request body after RoundTrip = %q, %v, want %q", rest, err, "hello")
	}
}

func TestRetryTransport_NoRetryOn500(t *testing.T) {
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "internal", http.StatusInternalServerError)
	}))
	defer srv.Close()

	resp, err := retryTestClient().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (500 must not be retried)", got)
	}
}

// Mutating calls (POST/PATCH) must not be retried on 5xx: the backend may
// have processed the original request, and a retry could duplicate the
// resource. 429 remains retryable for every method.
func TestRetryTransport_PostNotRetriedOn503(t *testing.T) {
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "bad gateway", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := retryTestClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (POST must not be retried on 503)", got)
	}
}

func TestRetryTransport_PostRetriedOn429(t *testing.T) {
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := retryTestClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2 (POST should be retried on 429)", got)
	}
}

func TestRetryTransport_UnbufferableBodyNotRetried(t *testing.T) {
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	big := strings.Repeat("x", retryMaxBodySize+1)
	req, err := http.NewRequest(http.MethodPost, srv.URL, struct{ io.Reader }{strings.NewReader(big)})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := retryTestClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 for a body larger than %d bytes", got, retryMaxBodySize)
	}
}

func TestRetryTransport_ContextCancelled(t *testing.T) {
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err = retryTestClient().Do(req)
	if err == nil {
		t.Fatal("expected context error while waiting to retry")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("wait not aborted by context, took %v", elapsed)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
}

func TestRetryDelay(t *testing.T) {
	if got := retryDelay(0, "2"); got != 2*time.Second {
		t.Errorf("retryDelay(0, \"2\") = %v, want 2s", got)
	}
	if got := retryDelay(0, "0"); got != 0 {
		t.Errorf("retryDelay(0, \"0\") = %v, want 0", got)
	}
	if got := retryDelay(0, "60"); got != 15*time.Second {
		t.Errorf("retryDelay(0, \"60\") = %v, want capped 15s", got)
	}
	future := time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)
	if got := retryDelay(0, future); got <= 0 || got > 3*time.Second {
		t.Errorf("retryDelay with HTTP-date = %v, want ~2s", got)
	}
	bases := []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second}
	for attempt, base := range bases {
		got := retryDelay(attempt, "")
		if got < base || got > base+100*time.Millisecond {
			t.Errorf("retryDelay(%d, \"\") = %v, want %v–%v (base + jitter)", attempt, got, base, base+100*time.Millisecond)
		}
	}
}
