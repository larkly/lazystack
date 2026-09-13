package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

func TestMicroversionBoundaries(t *testing.T) {
	for _, tc := range []struct{ a, b, want string }{
		{"1.999", "2.0", "1.999"}, {"2.10", "2.9", "2.9"},
		{"2.01", "2.1", "2.01"}, {"2.1.9", "2.1.1", "2.1.9"},
	} {
		t.Run(tc.a+"/"+tc.b, func(t *testing.T) {
			if got := minMicroversion(tc.a, tc.b); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
	for _, v := range []string{"2.", " 2.1", "2.1 ", "999999999999999999999999999999.1", "2.999999999999999999999999999999"} {
		t.Run(v, func(t *testing.T) {
			ma, mi, err := parseMicroversion(v)
			if err == nil || ma != 0 || mi != 0 {
				t.Fatalf("got (%d,%d,%v), want zero values and error", ma, mi, err)
			}
		})
	}
	// Characterize the parser's two-component comparison, not strict semver validation.
	for _, v := range []string{"02.001", "2.1.extra", "2.1.3.4"} {
		ma, mi, err := parseMicroversion(v)
		if err != nil || ma != 2 || mi != 1 {
			t.Errorf("parse %q = %d,%d,%v", v, ma, mi, err)
		}
	}
}

func TestNovaDiscoveryDocumentFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name, body, max, used string
		warning               bool
	}{
		{"empty document", `{}`, "unknown", "2.100", false},
		{"null document", `null`, "unknown", "2.100", false},
		{"unrelated version", `{"versions":[{"id":"v2.0","version":"2.80"}]}`, "unknown", "2.100", false},
		{"empty max", `{"versions":[{"id":"v2.1","version":""}]}`, "unknown", "2.100", false},
		{"singular fallback", `{"versions":[{"id":"v2.1","version":""}],"version":{"version":"2.90"}}`, "2.90", "2.90", true},
		{"array preferred", `{"versions":[{"id":"v2.1","version":"2.100"}],"version":{"version":"2.90"}}`, "2.100", "2.100", false},
		{"wrong field type", `{"versions":"invalid"}`, "unknown", "2.100", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc, closeFn := fakeComputeClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }))
			defer closeFn()
			max, used, warn := negotiateNovaMicroversion(context.Background(), sc)
			if max != tc.max || used != tc.used || (warn != "") != tc.warning {
				t.Fatalf("got (%q,%q,%q), want (%q,%q,warning=%v)", max, used, warn, tc.max, tc.used, tc.warning)
			}
			if tc.warning && (!strings.Contains(warn, tc.used) || !strings.Contains(warn, "2.100")) {
				t.Errorf("warning lacks versions: %s", warn)
			}
		})
	}
}

func TestNovaDiscoveryCancelledContext(t *testing.T) {
	sc, closeFn := fakeComputeClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("cancelled discovery reached server") }))
	defer closeFn()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	max, used, warn := negotiateNovaMicroversion(ctx, sc)
	if max != "unknown" || used != "2.100" || warn != "" {
		t.Fatalf("got %q,%q,%q", max, used, warn)
	}
}

func TestResolveMicroversionOverride(t *testing.T) {
	for _, tc := range []struct {
		override, max, used string
		calls               int
	}{
		{"2.88", "user-specified", "2.88", 0}, {"2.101", "user-specified", "2.100", 0},
		{"invalid", "2.95", "2.95", 1}, {"", "2.95", "2.95", 1},
	} {
		t.Run(tc.override, func(t *testing.T) {
			t.Setenv("OS_COMPUTE_API_VERSION", tc.override)
			calls := 0
			sc, closeFn := fakeComputeClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, `{"version":{"version":"2.95"}}`) }))
			defer closeFn()
			max, used, warn := resolveMicroversion(context.Background(), sc)
			if max != tc.max || used != tc.used || calls != tc.calls {
				t.Fatalf("got %q,%q,calls=%d", max, used, calls)
			}
			if tc.calls == 0 && warn != "" {
				t.Fatalf("override warning: %q", warn)
			}
		})
	}
}

func TestOptionalServiceFallbackChain(t *testing.T) {
	for _, success := range []int{1, 2, 3, 0} {
		t.Run(fmt.Sprint(success), func(t *testing.T) {
			var calls []gophercloud.EndpointOpts
			pc := &gophercloud.ProviderClient{}
			pc.EndpointLocator = func(eo gophercloud.EndpointOpts) (string, error) {
				calls = append(calls, eo)
				if len(calls) == success {
					return "https://storage.example/v/", nil
				}
				return "", fmt.Errorf("unavailable")
			}
			eo := gophercloud.EndpointOpts{Region: "RegionTwo", Availability: gophercloud.AvailabilityInternal}
			sc := tryBlockStorage(pc, eo)
			wantCalls := success
			if success == 0 {
				wantCalls = 3
			}
			if len(calls) != wantCalls {
				t.Fatalf("calls=%v want %d", calls, wantCalls)
			}
			// Gophercloud canonicalizes legacy volume to block-storage and
			// carries the legacy catalog names as aliases even for v1.
			types := []string{"block-storage", "block-storage", "block-storage"}
			versions := []int{3, 2, 1}
			for i, got := range calls {
				if got.Type != types[i] || got.Version != versions[i] || got.Region != eo.Region || got.Availability != eo.Availability {
					t.Errorf("lookup %d = %+v", i, got)
				}
			}
			if success == 0 {
				if sc != nil {
					t.Fatal("missing service must return nil")
				}
			} else if sc == nil || sc.Endpoint != "https://storage.example/v/" || sc.ProviderClient != pc {
				t.Fatalf("wrong service: %+v", sc)
			}
		})
	}
	for _, tc := range []struct {
		name, typ string
		fn        func(*gophercloud.ProviderClient, gophercloud.EndpointOpts) *gophercloud.ServiceClient
	}{
		{"load balancer", "load-balancer", tryLoadBalancer}, {"dns", "dns", tryDNS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, available := range []bool{true, false} {
				calls := 0
				pc := &gophercloud.ProviderClient{}
				pc.EndpointLocator = func(eo gophercloud.EndpointOpts) (string, error) {
					calls++
					if eo.Type != tc.typ || eo.Version != 2 || eo.Region != "R" {
						t.Errorf("lookup=%+v", eo)
					}
					if !available {
						return "", fmt.Errorf("not found")
					}
					return "https://optional.example/", nil
				}
				sc := tc.fn(pc, gophercloud.EndpointOpts{Region: "R"})
				if calls != 1 || (sc != nil) != available {
					t.Fatalf("available=%v calls=%d service=%+v", available, calls, sc)
				}
			}
		})
	}
}

// Isolate clouds.Parse from any credentials configured in the developer's shell.
func coverageCloudConfig(t *testing.T, authURL, extra string) {
	t.Helper()
	for _, env := range os.Environ() {
		key, _, _ := strings.Cut(env, "=")
		if strings.HasPrefix(key, "OS_") {
			t.Setenv(key, "")
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "clouds.yaml")
	content := fmt.Sprintf("clouds:\n  coverage:\n    auth_type: v3password\n    auth:\n      auth_url: %s/v3\n      username: test-user\n      password: test-password\n      user_domain_name: Default\n      project_name: old-project\n      project_id: old-id\n%s", authURL, extra)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OS_CLIENT_CONFIG_FILE", path)
}

func TestConnectWithProjectSendsReplacementScope(t *testing.T) {
	for _, extra := range []string{"", "      trust_id: old-trust\n"} {
		t.Run(extra, func(t *testing.T) {
			var request map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/v3/auth/tokens" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				// Stop after capturing real auth serialization; no external catalog required.
				http.Error(w, "denied", http.StatusUnauthorized)
			}))
			defer srv.Close()
			coverageCloudConfig(t, srv.URL, extra)
			client, err := ConnectWithProject(context.Background(), "coverage", "target-id")
			if client != nil || err == nil || !strings.Contains(err.Error(), `authenticating to "coverage"`) {
				t.Fatalf("got client=%v err=%v", client, err)
			}
			auth, ok := request["auth"].(map[string]any)
			if !ok {
				t.Fatalf("missing auth request: %v", request)
			}
			want := map[string]any{"project": map[string]any{"id": "target-id"}}
			if !reflect.DeepEqual(auth["scope"], want) {
				t.Fatalf("scope=%#v want %#v", auth["scope"], want)
			}
		})
	}
}

func TestConnectWithProjectRejectsFixedScopeCredentials(t *testing.T) {
	for _, extra := range []string{"      token: fixed-token\n", "      application_credential_id: app-id\n      application_credential_secret: secret\n", "      application_credential_name: app-name\n      application_credential_secret: secret\n"} {
		t.Run(strings.Fields(extra)[0], func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("fixed-scope credentials must not reach authentication")
				http.Error(w, "unexpected", 500)
			}))
			defer srv.Close()
			coverageCloudConfig(t, srv.URL, extra)
			client, err := ConnectWithProject(context.Background(), "coverage", "target-id")
			if client != nil || err == nil || !strings.Contains(err.Error(), "re-scoping is not possible") || !strings.Contains(err.Error(), "target-id") {
				t.Fatalf("got client=%v err=%v", client, err)
			}
		})
	}
}
