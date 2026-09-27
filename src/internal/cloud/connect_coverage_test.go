package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConnectWithProjectCatalogRequirements(t *testing.T) {
	for _, tc := range []struct {
		name      string
		services  []string
		wantError string
	}{
		{"compute required", nil, "compute client:"},
		{"image required", []string{"compute"}, "image client:"},
		{"network required", []string{"compute", "image"}, "network client:"},
		{"optional services absent", []string{"compute", "image", "network"}, ""},
		{"optional services present", []string{"compute", "image", "network", "volumev3", "load-balancer", "dns"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var endpoint string
			discoveries := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v3/auth/tokens":
					var request struct {
						Auth struct {
							Scope struct {
								Project struct {
									ID string `json:"id"`
								} `json:"project"`
							} `json:"scope"`
						} `json:"auth"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if request.Auth.Scope.Project.ID != "target-id" {
						t.Errorf("scope ID=%q", request.Auth.Scope.Project.ID)
					}
					catalog := []map[string]any{}
					for _, typ := range tc.services {
						catalog = append(catalog, map[string]any{"type": typ, "name": typ, "endpoints": []map[string]any{{"interface": "public", "region": "RegionOne", "url": endpoint + "/" + typ + "/"}}})
					}
					w.Header().Set("X-Subject-Token", "coverage-token")
					w.WriteHeader(http.StatusCreated)
					if err := json.NewEncoder(w).Encode(map[string]any{"token": map[string]any{"expires_at": "2099-01-01T00:00:00Z", "catalog": catalog, "project": map[string]string{"id": "target-id"}}}); err != nil {
						t.Error(err)
					}
				case r.Method == http.MethodGet && r.URL.Path == "/compute/":
					discoveries++
					if got := r.Header.Get("X-Auth-Token"); got != "coverage-token" {
						t.Errorf("discovery token=%q", got)
					}
					if r.Header.Get("X-OpenStack-Nova-API-Version") != "" || r.Header.Get("OpenStack-API-Version") != "" {
						t.Error("discovery sent a microversion header")
					}
					fmt.Fprint(w, `{"version":{"id":"v2.1","version":"2.88"}}`)
				case r.Method == http.MethodGet && (r.URL.Path == "/image/" || r.URL.Path == "/network/" || r.URL.Path == "/volumev3/" || r.URL.Path == "/load-balancer/" || r.URL.Path == "/dns/"):
					version := "v2.0"
					if r.URL.Path == "/volumev3/" {
						version = "v3.0"
					}
					fmt.Fprintf(w, `{"versions":[{"id":%q,"status":"CURRENT","links":[{"rel":"self","href":%q}]}]}`, version, endpoint+r.URL.Path+version+"/")
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			endpoint = srv.URL
			coverageCloudConfig(t, srv.URL, "")
			client, err := ConnectWithProject(context.Background(), "coverage", "target-id")
			if tc.wantError != "" {
				if client != nil || err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("client=%+v err=%v want %q", client, err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if client == nil || client.Compute == nil || client.Image == nil || client.Network == nil || client.ProviderClient == nil {
				t.Fatalf("missing required clients: %+v", client)
			}
			if client.CloudName != "coverage" || client.Region != "RegionOne" || client.ProviderClient.TokenID != "coverage-token" {
				t.Errorf("wrong identity: %+v", client)
			}
			// The SDK discovers the versionless catalog endpoint first;
			// our microversion negotiation then fetches its supported range.
			if discoveries != 2 || client.NovaMicroversionMax != "2.88" || client.NovaMicroversionUsed != "2.88" || client.Compute.Microversion != "2.88" {
				t.Errorf("negotiation not propagated: %+v discoveries=%d", client, discoveries)
			}
			optional := tc.name == "optional services present"
			if (client.BlockStorage != nil) != optional || (client.LoadBalancer != nil) != optional || (client.DNS != nil) != optional {
				t.Errorf("optional services present=%v: %+v", optional, client)
			}
		})
	}
}
