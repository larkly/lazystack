package network

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

// fakeRevisionedPort is a single Neutron port that enforces the
// standard-attr-revisions If-Match precondition on PUT.
type fakeRevisionedPort struct {
	mu       sync.Mutex
	fixedIPs []map[string]string
	revision int
	getErr   bool
	// beforePut runs on every PUT before the precondition check; it may
	// mutate the port to simulate a concurrent writer.
	beforePut func(p *fakeRevisionedPort, attempt int)
	puts      []string // raw PUT bodies that were accepted
	putTries  int
	ifMatch   []string
}

func (f *fakeRevisionedPort) client(t *testing.T) *gophercloud.ServiceClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !strings.HasSuffix(r.URL.Path, "/ports/port-1") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if f.getErr {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case http.MethodPut:
			f.putTries++
			if f.beforePut != nil {
				f.beforePut(f, f.putTries)
			}
			im := r.Header.Get("If-Match")
			f.ifMatch = append(f.ifMatch, im)
			if im != "" && im != fmt.Sprintf("revision_number=%d", f.revision) {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			raw, _ := io.ReadAll(r.Body)
			var body struct {
				Port struct {
					FixedIPs *[]map[string]string `json:"fixed_ips"`
				} `json:"port"`
			}
			if err := json.Unmarshal(raw, &body); err != nil || body.Port.FixedIPs == nil {
				// Neutron rejects fixed_ips:null.
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.puts = append(f.puts, string(raw))
			f.fixedIPs = *body.Port.FixedIPs
			f.revision++
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"port": map[string]any{
			"id":              "port-1",
			"fixed_ips":       f.fixedIPs,
			"revision_number": f.revision,
		}})
	}))
	t.Cleanup(srv.Close)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{HTTPClient: *srv.Client()},
		Endpoint:       srv.URL + "/",
	}
}

func ipsOf(f *fakeRevisionedPort) string {
	var parts []string
	for _, ip := range f.fixedIPs {
		parts = append(parts, ip["subnet_id"]+"="+ip["ip_address"])
	}
	return strings.Join(parts, ",")
}

func TestRemoveFixedIPFromPortLastAddressSendsEmptyArray(t *testing.T) {
	f := &fakeRevisionedPort{revision: 4, fixedIPs: []map[string]string{{"subnet_id": "sub-a", "ip_address": "10.0.0.1"}}}
	if err := RemoveFixedIPFromPort(context.Background(), f.client(t), "port-1", "sub-a"); err != nil {
		t.Fatalf("RemoveFixedIPFromPort: %v", err)
	}
	if len(f.puts) != 1 || !strings.Contains(f.puts[0], `"fixed_ips":[]`) {
		t.Fatalf("PUT bodies = %q, want fixed_ips:[]", f.puts)
	}
	if f.ifMatch[0] != "revision_number=4" {
		t.Errorf("If-Match = %q, want revision_number=4", f.ifMatch[0])
	}
}

func TestRemoveFixedIPFromPortNoMatchPreservesAddresses(t *testing.T) {
	f := &fakeRevisionedPort{revision: 1, fixedIPs: []map[string]string{
		{"subnet_id": "sub-a", "ip_address": "10.0.0.1"},
		{"subnet_id": "sub-b", "ip_address": "10.1.0.1"},
	}}
	if err := RemoveFixedIPFromPort(context.Background(), f.client(t), "port-1", "sub-z"); err == nil {
		t.Fatal("expected an error for a subnet not on the port")
	}
	if f.putTries != 0 || ipsOf(f) != "sub-a=10.0.0.1,sub-b=10.1.0.1" {
		t.Fatalf("port mutated: puts=%d ips=%s", f.putTries, ipsOf(f))
	}
}

func TestRemoveFixedIPFromPortGetErrorPreventsPut(t *testing.T) {
	f := &fakeRevisionedPort{getErr: true, fixedIPs: []map[string]string{{"subnet_id": "sub-a", "ip_address": "10.0.0.1"}}}
	if err := RemoveFixedIPFromPort(context.Background(), f.client(t), "port-1", "sub-a"); err == nil {
		t.Fatal("expected GET failure to be returned")
	}
	if f.putTries != 0 {
		t.Fatalf("PUT sent after failed GET (%d)", f.putTries)
	}
}

func TestAddFixedIPToPortRefreshesCurrentAddresses(t *testing.T) {
	// The port gained sub-b after the caller last looked at it; the add must
	// be based on the live port, not on a stale snapshot.
	f := &fakeRevisionedPort{revision: 7, fixedIPs: []map[string]string{
		{"subnet_id": "sub-a", "ip_address": "10.0.0.1"},
		{"subnet_id": "sub-b", "ip_address": "10.1.0.1"},
	}}
	if err := AddFixedIPToPort(context.Background(), f.client(t), "port-1", "sub-c", "10.2.0.1"); err != nil {
		t.Fatalf("AddFixedIPToPort: %v", err)
	}
	if got := ipsOf(f); got != "sub-a=10.0.0.1,sub-b=10.1.0.1,sub-c=10.2.0.1" {
		t.Fatalf("fixed IPs = %s", got)
	}
	if f.ifMatch[0] != "revision_number=7" {
		t.Errorf("If-Match = %q, want revision_number=7", f.ifMatch[0])
	}
}

func TestAddFixedIPToPortRetriesOnConcurrentRevision(t *testing.T) {
	f := &fakeRevisionedPort{revision: 1, fixedIPs: []map[string]string{{"subnet_id": "sub-a", "ip_address": "10.0.0.1"}}}
	f.beforePut = func(p *fakeRevisionedPort, attempt int) {
		if attempt == 1 {
			// A concurrent writer adds sub-d between our GET and PUT.
			p.fixedIPs = append(p.fixedIPs, map[string]string{"subnet_id": "sub-d", "ip_address": "10.3.0.1"})
			p.revision++
		}
	}
	if err := AddFixedIPToPort(context.Background(), f.client(t), "port-1", "sub-c", "10.2.0.1"); err != nil {
		t.Fatalf("AddFixedIPToPort: %v", err)
	}
	if got := ipsOf(f); got != "sub-a=10.0.0.1,sub-d=10.3.0.1,sub-c=10.2.0.1" {
		t.Fatalf("concurrent address lost: %s", got)
	}
	if f.putTries != 2 {
		t.Errorf("PUT attempts = %d, want 2", f.putTries)
	}
}

func TestAddFixedIPToPortPersistentConflictIsBounded(t *testing.T) {
	f := &fakeRevisionedPort{revision: 1, fixedIPs: []map[string]string{{"subnet_id": "sub-a", "ip_address": "10.0.0.1"}}}
	f.beforePut = func(p *fakeRevisionedPort, _ int) { p.revision++ }
	err := AddFixedIPToPort(context.Background(), f.client(t), "port-1", "sub-c", "10.2.0.1")
	if err == nil {
		t.Fatal("expected a conflict error")
	}
	if len(f.puts) != 0 || ipsOf(f) != "sub-a=10.0.0.1" {
		t.Fatalf("port overwritten: %s", ipsOf(f))
	}
	if f.putTries < 2 || f.putTries > 5 {
		t.Errorf("PUT attempts = %d, want a small bounded retry", f.putTries)
	}
}

func TestAddFixedIPToPortSkipsDuplicate(t *testing.T) {
	f := &fakeRevisionedPort{revision: 1, fixedIPs: []map[string]string{{"subnet_id": "sub-a", "ip_address": "10.0.0.1"}}}
	if err := AddFixedIPToPort(context.Background(), f.client(t), "port-1", "sub-a", "10.0.0.1"); err != nil {
		t.Fatalf("AddFixedIPToPort: %v", err)
	}
	if f.putTries != 0 || ipsOf(f) != "sub-a=10.0.0.1" {
		t.Fatalf("duplicate address written: puts=%d ips=%s", f.putTries, ipsOf(f))
	}
}
