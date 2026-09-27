package loadbalancer

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/monitors"
	"github.com/larkly/lazystack/internal/testutil"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCreatePoolMonitorLifecycleHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, poolStatus            string
		monitorStatus, deleteStatus int
		wantErr                     string
	}{
		{"success", "ACTIVE", 201, 0, ""},
		{"cleanup", "ACTIVE", 400, 204, "creating health monitor for pool pool"},
		{"already gone", "ACTIVE", 400, 404, "creating health monitor for pool pool"},
		{"cleanup fails", "ACTIVE", 400, 500, "cleanup failed"},
		{"pool error", "ERROR_CREATE", 0, 204, "waiting for pool pool to become ACTIVE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.Method+" "+r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "POST /lbaas/pools":
					var body map[string]map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if !reflect.DeepEqual(body["pool"], map[string]any{"loadbalancer_id": "lb", "name": "web", "protocol": "HTTP", "lb_algorithm": "ROUND_ROBIN"}) {
						t.Errorf("body=%v", body)
					}
					w.WriteHeader(201)
					w.Write([]byte(`{"pool":{"id":"pool","name":"web","protocol":"HTTP","lb_algorithm":"ROUND_ROBIN","admin_state_up":true}}`))
				case "GET /lbaas/pools/pool":
					json.NewEncoder(w).Encode(map[string]any{"pool": map[string]string{"id": "pool", "provisioning_status": tc.poolStatus}})
				case "POST /lbaas/healthmonitors":
					var body map[string]map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["healthmonitor"]["pool_id"] != "pool" || body["healthmonitor"]["type"] != "HTTP" || body["healthmonitor"]["delay"] != float64(5) {
						t.Errorf("body=%v", body)
					}
					w.WriteHeader(tc.monitorStatus)
					w.Write([]byte(`{"healthmonitor":{"id":"monitor"}}`))
				case "DELETE /lbaas/pools/pool":
					w.WriteHeader(tc.deleteStatus)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer close()
			mon := monitors.CreateOpts{PoolID: "unchanged", Type: "HTTP", Delay: 5, Timeout: 2, MaxRetries: 3}
			got, err := CreatePool(context.Background(), client, "lb", "web", "HTTP", "ROUND_ROBIN", &mon)
			if mon.PoolID != "unchanged" {
				t.Fatal("mutated caller options")
			}
			want := []string{"POST /lbaas/pools", "GET /lbaas/pools/pool"}
			if tc.monitorStatus != 0 {
				want = append(want, "POST /lbaas/healthmonitors")
			}
			if tc.deleteStatus != 0 {
				want = append(want, "DELETE /lbaas/pools/pool")
			}
			if !reflect.DeepEqual(calls, want) {
				t.Errorf("calls=%v want=%v", calls, want)
			}
			if tc.wantErr == "" {
				if err != nil || got == nil || got.ID != "pool" || got.MonitorID != "monitor" || !got.AdminStateUp {
					t.Fatalf("got=%+v err=%v", got, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) || got != nil {
				t.Fatalf("got=%v err=%v", got, err)
			}
			if tc.deleteStatus == 404 {
				if strings.Contains(err.Error(), "cleanup failed") {
					t.Fatal(err)
				}
			}
		})
	}
}
func TestWaitForActiveDeadlineAndCancellationHTTP(t *testing.T) {
	for _, kind := range []string{"deadline", "cancel", "unauthorized"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != "/lbaas/loadbalancers/lb" {
					t.Errorf("request=%s %s", r.Method, r.URL.Path)
				}
				if kind == "unauthorized" {
					w.WriteHeader(401)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"loadbalancer":{"provisioning_status":"PENDING_UPDATE"}}`))
				if kind == "cancel" {
					cancel()
				}
			}))
			defer close()
			timeout := time.Minute
			if kind == "deadline" {
				timeout = 0
			}
			err := WaitForActive(ctx, client, "lb", timeout)
			if err == nil {
				t.Fatal("expected error")
			}
			switch kind {
			case "deadline":
				if calls != 0 || !strings.Contains(err.Error(), "timed out") {
					t.Fatalf("calls=%d err=%v", calls, err)
				}
			case "cancel":
				if calls != 1 || !errors.Is(err, context.Canceled) {
					t.Fatalf("calls=%d err=%v", calls, err)
				}
			case "unauthorized":
				if calls != 1 || !strings.Contains(err.Error(), "polling LB lb") {
					t.Fatalf("calls=%d err=%v", calls, err)
				}
			}
		})
	}
}
