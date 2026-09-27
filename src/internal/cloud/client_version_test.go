package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// versionCloud is a keystone + nova stub for exercising Connect end to end.
type versionCloud struct {
	srv *httptest.Server

	// discovery answers GET on the compute root; nil means a 2.100 document.
	discovery func(w http.ResponseWriter)
	// maxMinor makes /servers reject newer 2.x headers with 406, like an
	// older Nova. Zero means accept anything.
	maxMinor int
	// computeRegions lists the regions the catalog offers compute in, in
	// catalog order. Each region gets its own compute URL.
	computeRegions []string

	mu            sync.Mutex
	serverHeaders []string
	rootGets      int
}

func newVersionCloud(t *testing.T, configure func(*versionCloud)) *versionCloud {
	t.Helper()
	c := &versionCloud{computeRegions: []string{"RegionOne"}}
	if configure != nil {
		configure(c)
	}
	c.srv = httptest.NewServer(http.HandlerFunc(c.serve(t)))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *versionCloud) serve(t *testing.T) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v3/auth/tokens":
			var endpoints []map[string]any
			for _, region := range c.computeRegions {
				endpoints = append(endpoints, map[string]any{"interface": "public", "region": region, "region_id": region, "url": c.srv.URL + "/compute-" + region + "/"})
			}
			catalog := []map[string]any{{"type": "compute", "name": "nova", "endpoints": endpoints}}
			for _, typ := range []string{"image", "network"} {
				var eps []map[string]any
				for _, region := range c.computeRegions {
					eps = append(eps, map[string]any{"interface": "public", "region": region, "region_id": region, "url": c.srv.URL + "/" + typ + "-" + region + "/"})
				}
				catalog = append(catalog, map[string]any{"type": typ, "name": typ, "endpoints": eps})
			}
			w.Header().Set("X-Subject-Token", "tok")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"token": map[string]any{"expires_at": "2099-01-01T00:00:00Z", "catalog": catalog}})
		case strings.HasPrefix(r.URL.Path, "/compute-") && strings.HasSuffix(r.URL.Path, "/servers/detail"):
			h := r.Header.Get("X-OpenStack-Nova-API-Version")
			c.mu.Lock()
			c.serverHeaders = append(c.serverHeaders, h)
			c.mu.Unlock()
			if c.maxMinor > 0 {
				ma, mi, err := parseMicroversion(h)
				if h != "" && (err != nil || ma != 2 || mi > c.maxMinor) {
					w.WriteHeader(http.StatusNotAcceptable)
					fmt.Fprint(w, `{"computeFault":{"message":"Version not supported"}}`)
					return
				}
			}
			fmt.Fprint(w, `{"servers":[]}`)
		case strings.HasPrefix(r.URL.Path, "/compute-"):
			// The SDK's own endpoint discovery comes first; only the
			// second request is lazystack's microversion negotiation.
			c.mu.Lock()
			c.rootGets++
			sdk := c.rootGets == 1
			c.mu.Unlock()
			if c.discovery != nil && !sdk {
				c.discovery(w)
				return
			}
			fmt.Fprint(w, `{"version":{"id":"v2.1","version":"2.100"}}`)
		case strings.HasPrefix(r.URL.Path, "/image-") || strings.HasPrefix(r.URL.Path, "/network-"):
			fmt.Fprintf(w, `{"versions":[{"id":"v2.0","status":"CURRENT","links":[{"rel":"self","href":%q}]}]}`, c.srv.URL+r.URL.Path+"v2.0/")
		default:
			http.NotFound(w, r)
		}
	}
}

func (c *versionCloud) connect(t *testing.T, extra string) *Client {
	t.Helper()
	coverageCloudConfig(t, c.srv.URL, extra)
	client, err := Connect(context.Background(), "coverage")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return client
}

// listServers issues a real follow-up compute request and returns the
// microversion header the server saw and the request error.
func (c *versionCloud) listServers(t *testing.T, client *Client) (string, error) {
	t.Helper()
	_, err := client.Compute.Get(context.Background(), client.Compute.ServiceURL("servers", "detail"), nil, nil)
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.serverHeaders) == 0 {
		t.Fatal("no servers request reached the stub")
	}
	return c.serverHeaders[len(c.serverHeaders)-1], err
}

func TestParseMicroversionRejectsNonNovaForms(t *testing.T) {
	for _, v := range []string{
		"2.1.9", "2.1.5", "2.1.extra", "-2.1", "2.-1", "+2.1", "2.+1",
		" 2.1", "2.1 ", "\t2.1", "v2.1", "02.1", "2.01", "02.001", "2..1", "latest",
	} {
		if ma, mi, err := parseMicroversion(v); err == nil {
			t.Errorf("parseMicroversion(%q) = %d.%d, want error", v, ma, mi)
		}
	}
	for v, want := range map[string][2]int{"2.1": {2, 1}, "2.0": {2, 0}, "2.100": {2, 100}, "3.0": {3, 0}} {
		ma, mi, err := parseMicroversion(v)
		if err != nil || ma != want[0] || mi != want[1] {
			t.Errorf("parseMicroversion(%q) = %d,%d,%v", v, ma, mi, err)
		}
	}
}

func TestResolveMicroversionRejectsMalformedOverride(t *testing.T) {
	for _, override := range []string{"2.1.9", "-2.60", "v2.60", " 2.60", "2.60 ", "+2.60", "2.0", "1.5"} {
		t.Run(override, func(t *testing.T) {
			t.Setenv("OS_COMPUTE_API_VERSION", override)
			calls := 0
			sc, closeFn := fakeComputeClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				fmt.Fprint(w, `{"version":{"version":"2.95"}}`)
			}))
			defer closeFn()
			max, used, warn := resolveMicroversion(context.Background(), sc)
			if max != "2.95" || used != "2.95" || calls != 1 {
				t.Fatalf("got max=%q used=%q calls=%d, want the negotiated 2.95", max, used, calls)
			}
			if !strings.Contains(warn, "OS_COMPUTE_API_VERSION") {
				t.Errorf("warning %q does not mention the ignored override", warn)
			}
		})
	}
}

func TestNegotiateMalformedDiscoveredVersionFallsBackToBase(t *testing.T) {
	for _, body := range []string{
		`{"version":{"id":"v2.1","version":"v2.90"}}`,
		`{"version":{"id":"v2.1","version":"2.90.1"}}`,
		`{"version":{"id":"v2.1","version":"banana"}}`,
		`{"versions":[{"id":"v2.1","version":" 2.90"}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			sc, closeFn := fakeComputeClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer closeFn()
			_, used, warn := negotiateNovaMicroversion(context.Background(), sc)
			if used != "2.1" {
				t.Errorf("used = %q, want base 2.1", used)
			}
			if warn == "" {
				t.Error("expected a degradation warning")
			}
		})
	}
}

func TestConnectSendsOverrideMicroversionOnLaterRequests(t *testing.T) {
	c := newVersionCloud(t, nil)
	coverageCloudConfig(t, c.srv.URL, "") // clears every OS_* variable
	t.Setenv("OS_COMPUTE_API_VERSION", "2.60")
	client, err := Connect(context.Background(), "coverage")
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.listServers(t, client)
	if err != nil {
		t.Fatal(err)
	}
	if h != "2.60" {
		t.Errorf("servers request header = %q, want 2.60", h)
	}
	if client.CapabilityWarning != "" {
		t.Errorf("explicit valid override should not warn: %q", client.CapabilityWarning)
	}
}

func TestConnectDiscoveryFailureUsesBaseMicroversion(t *testing.T) {
	c := newVersionCloud(t, func(c *versionCloud) {
		c.discovery = func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) }
		c.maxMinor = 60 // an older Nova that rejects anything newer
	})
	client := c.connect(t, "")
	if client.NovaMicroversionUsed != "2.1" || client.Compute.Microversion != "2.1" {
		t.Errorf("used=%q compute=%q, want base 2.1", client.NovaMicroversionUsed, client.Compute.Microversion)
	}
	h, err := c.listServers(t, client)
	if err != nil {
		t.Fatalf("older cloud rejected server list after discovery failure: %v", err)
	}
	if h != "2.1" {
		t.Errorf("servers request header = %q, want 2.1", h)
	}
	if client.CapabilityWarning == "" {
		t.Error("failed discovery must produce a visible capability warning")
	}
}

func TestConnectCapabilityWarning(t *testing.T) {
	for _, tc := range []struct {
		name, discovered string
		wantWarning      bool
	}{
		{"at ceiling", "2.100", false},
		{"above ceiling", "2.150", false},
		{"older nova", "2.88", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newVersionCloud(t, func(c *versionCloud) {
				c.discovery = func(w http.ResponseWriter) {
					fmt.Fprintf(w, `{"version":{"id":"v2.1","version":%q}}`, tc.discovered)
				}
			})
			client := c.connect(t, "")
			if (client.CapabilityWarning != "") != tc.wantWarning {
				t.Errorf("CapabilityWarning = %q, want warning=%v", client.CapabilityWarning, tc.wantWarning)
			}
			if tc.wantWarning {
				for _, s := range []string{"tok", "test-password", c.srv.URL} {
					if strings.Contains(client.CapabilityWarning, s) {
						t.Errorf("warning leaks %q: %s", s, client.CapabilityWarning)
					}
				}
			}
		})
	}
}

func TestConnectRegionFromSelectedEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name       string
		regions    []string
		configured string
		want       string
	}{
		{"single catalog region", []string{"RegionTwo"}, "", "RegionTwo"},
		{"first of many", []string{"west", "east"}, "", "west"},
		{"configured region kept", []string{"west", "east"}, "east", "east"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newVersionCloud(t, func(c *versionCloud) { c.computeRegions = tc.regions })
			extra := ""
			if tc.configured != "" {
				extra = "    region_name: " + tc.configured + "\n"
			}
			client := c.connect(t, extra)
			if client.Region != tc.want {
				t.Errorf("Region = %q, want %q", client.Region, tc.want)
			}
			if !strings.Contains(client.Compute.Endpoint, "/compute-"+tc.want+"/") {
				t.Errorf("compute endpoint %s does not match displayed region %s", client.Compute.Endpoint, tc.want)
			}
		})
	}
}

func TestResolveRegionFallsBackToAuto(t *testing.T) {
	if got := resolveRegion(nil, "", ""); got != "auto" {
		t.Errorf("resolveRegion without catalog = %q, want auto", got)
	}
}
