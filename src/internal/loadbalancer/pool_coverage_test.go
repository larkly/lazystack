package loadbalancer

import (
	"context"
	"errors"
	"github.com/gophercloud/gophercloud/v2"
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
		// 409 is retried and covered by TestCreatePoolCleanupRetriesConflictHTTP.
		for _, cleanup := range []int{204, 404, 500} {
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
				var ce *PoolCleanupError
				if cleanup == 500 {
					if !errors.As(err, &ce) || ce.PoolID != "p" || !strings.Contains(err.Error(), "cleanup failed") || !strings.Contains(err.Error(), "500") {
						t.Fatalf("cleanup failure not reported: %v", err)
					}
				} else if errors.As(err, &ce) || strings.Contains(err.Error(), "cleanup failed") {
					t.Fatalf("cleanup %d must count as done: %v", cleanup, err)
				}
			})
		}
	}
}

// fastPoolCleanup removes the retry delay between pool cleanup attempts.
func fastPoolCleanup(t *testing.T) {
	t.Helper()
	old := poolCleanupInterval
	poolCleanupInterval = 0
	t.Cleanup(func() { poolCleanupInterval = old })
}

func TestCreatePoolCleanupRetriesConflictHTTP(t *testing.T) {
	fastPoolCleanup(t)
	for _, tc := range []struct {
		name      string
		conflicts int // DELETE answers 409 this many times, then 204
		wantLeft  bool
	}{
		{"conflict then deleted", 2, false},
		{"conflict never resolves", 1 << 30, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deletes := 0
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "POST /lbaas/pools":
					w.WriteHeader(201)
					w.Write([]byte(`{"pool":{"id":"p"}}`))
				case "GET /lbaas/pools/p":
					w.WriteHeader(403)
				case "DELETE /lbaas/pools/p":
					deletes++
					if deletes <= tc.conflicts {
						w.WriteHeader(409)
						return
					}
					w.WriteHeader(204)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
			}))
			defer close()
			_, err := CreatePool(context.Background(), client, "lb", "web", "HTTP", "ROUND_ROBIN", &monitors.CreateOpts{Type: "HTTP"})
			if err == nil || !strings.Contains(err.Error(), "waiting for pool p to become ACTIVE") || !gophercloud.ResponseCodeIs(err, 403) {
				t.Fatalf("original failure lost: %v", err)
			}
			var ce *PoolCleanupError
			if !tc.wantLeft {
				if deletes != tc.conflicts+1 || errors.As(err, &ce) {
					t.Fatalf("deletes=%d err=%v", deletes, err)
				}
				return
			}
			if deletes != poolCleanupAttempts {
				t.Fatalf("deletes=%d, want bounded at %d", deletes, poolCleanupAttempts)
			}
			if !errors.As(err, &ce) || ce.PoolID != "p" || !gophercloud.ResponseCodeIs(ce.CleanupErr, 409) || !strings.Contains(err.Error(), "pool p may remain") {
				t.Fatalf("unresolved cleanup not reported: %v", err)
			}
		})
	}
}

func TestCreatePoolCleanupSurvivesCancelledContextHTTP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	deleted := false
	client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /lbaas/pools":
			w.WriteHeader(201)
			w.Write([]byte(`{"pool":{"id":"p"}}`))
		case "GET /lbaas/pools/p":
			cancel() // caller gives up while waiting for the pool
			w.WriteHeader(500)
		case "DELETE /lbaas/pools/p":
			deleted = true
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer close()
	_, err := CreatePool(ctx, client, "lb", "web", "HTTP", "ROUND_ROBIN", &monitors.CreateOpts{Type: "HTTP"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
	if !deleted {
		t.Fatal("pool was not rolled back after the caller context was cancelled")
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
