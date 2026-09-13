package loadbalancer

import (
	"context"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/monitors"
	"github.com/larkly/lazystack/internal/testutil"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCreatePoolWithoutMonitorHTTP(t *testing.T) {
	for _, status := range []int{201, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.URL.Path != "/lbaas/pools" {
					t.Errorf("request=%s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				w.Write([]byte(`{"pool":{"id":"p","name":"web","protocol":"HTTP","lb_algorithm":"ROUND_ROBIN"}}`))
			}))
			defer close()
			got, err := CreatePool(context.Background(), client, "lb", "web", "HTTP", "ROUND_ROBIN", nil)
			if calls != 1 {
				t.Fatalf("calls=%d; no monitor must not poll", calls)
			}
			if status == 403 {
				if err == nil || got != nil || !strings.Contains(err.Error(), "creating pool") {
					t.Fatalf("got=%v err=%v", got, err)
				}
				return
			}
			if err != nil || got == nil || got.ID != "p" || got.Name != "web" || got.Protocol != "HTTP" || got.LBMethod != "ROUND_ROBIN" || got.MonitorID != "" {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}
func TestCreatePoolPollingFailureCleanupHTTP(t *testing.T) {
	for _, status := range []int{401, 403, 404} {
		for _, cleanup := range []int{204, 404, 409, 500} {
			t.Run(http.StatusText(status)+"/cleanup/"+http.StatusText(cleanup), func(t *testing.T) {
				calls := 0
				client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.Header().Set("Content-Type", "application/json")
					switch calls {
					case 1:
						if r.Method != "POST" || r.URL.Path != "/lbaas/pools" {
							t.Errorf("request=%s %s", r.Method, r.URL.Path)
						}
						w.WriteHeader(201)
						w.Write([]byte(`{"pool":{"id":"p"}}`))
					case 2:
						if r.Method != "GET" || r.URL.Path != "/lbaas/pools/p" {
							t.Errorf("request=%s %s", r.Method, r.URL.Path)
						}
						w.WriteHeader(status)
					case 3:
						if r.Method != "DELETE" || r.URL.Path != "/lbaas/pools/p" {
							t.Errorf("request=%s %s", r.Method, r.URL.Path)
						}
						w.WriteHeader(cleanup)
					default:
						t.Errorf("extra request %s %s", r.Method, r.URL.Path)
						w.WriteHeader(500)
					}
				}))
				defer close()
				got, err := CreatePool(context.Background(), client, "lb", "web", "HTTP", "ROUND_ROBIN", &monitors.CreateOpts{Type: "HTTP", Delay: 5, Timeout: 2, MaxRetries: 3})
				if got != nil || err == nil || !strings.Contains(err.Error(), "waiting for pool p to become ACTIVE") || calls != 3 {
					t.Fatalf("got=%v err=%v calls=%d", got, err, calls)
				}
				if status == 404 && !strings.Contains(err.Error(), "pool p not found") {
					t.Fatal(err)
				}
			})
		}
	}
}
func TestWaitForActivePendingTimesOutHTTP(t *testing.T) {
	calls := 0
	client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"loadbalancer":{"provisioning_status":"PENDING_CREATE"}}`))
	}))
	defer close()
	err := WaitForActive(context.Background(), client, "lb", 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for LB lb") || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
