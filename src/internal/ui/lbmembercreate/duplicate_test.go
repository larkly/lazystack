package lbmembercreate

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/testutil"
)

var existingMembers = []loadbalancer.Member{
	{ID: "m1", Address: "10.0.0.5", ProtocolPort: 80},
	{ID: "m2", Address: "2001:db8::10", ProtocolPort: 443},
}

func manualForm(addr, port string) Model {
	m := New(nil, nil, "pool-1", "web", "10.0.0.1", existingMembers)
	m.addressSource = addressSourceIP
	m.addrInput.SetValue(addr)
	m.portInput.SetValue(port)
	return m
}

func TestManualDuplicateAddressAndPortRejected(t *testing.T) {
	for _, tc := range []struct{ addr, port string }{
		{"10.0.0.5", "80"},
		{"2001:DB8:0::10", "443"}, // same IPv6 address, different spelling
	} {
		m, cmd := manualForm(tc.addr, tc.port).submit()
		if cmd != nil || !strings.Contains(m.err, "already") {
			t.Errorf("%s:%s: cmd=%v err=%q, want duplicate warning", tc.addr, tc.port, cmd != nil, m.err)
		}
	}
}

func TestSameAddressDifferentPortAllowed(t *testing.T) {
	for _, tc := range []struct{ addr, port string }{
		{"10.0.0.5", "8080"},
		{"2001:db8::10", "80"},
		{"10.0.0.6", "80"},
	} {
		m, cmd := manualForm(tc.addr, tc.port).submit()
		if cmd == nil || m.err != "" {
			t.Errorf("%s:%s rejected: %q", tc.addr, tc.port, m.err)
		}
	}
}

func TestServerSourceUsesAddressAndPort(t *testing.T) {
	m := New(nil, nil, "pool-1", "web", "10.0.0.1", existingMembers)
	// A server that already backs the pool on port 80 stays selectable: it
	// can be added again on another port.
	m, _ = m.Update(memberServersLoadedMsg{servers: []memberServerOption{{id: "srv-1", name: "web-1", address: "10.0.0.5", status: "ACTIVE"}}})
	m.addressSource = addressSourceServer
	if _, ok := m.selectedServer(); !ok {
		t.Fatal("server with an existing member address is not selectable")
	}

	m.portInput.SetValue("80")
	if m2, cmd := m.submit(); cmd != nil || !strings.Contains(m2.err, "already") {
		t.Fatalf("server duplicate: cmd=%v err=%q, want duplicate warning", cmd != nil, m2.err)
	}
	m.portInput.SetValue("8080")
	if m2, cmd := m.submit(); cmd == nil || m2.err != "" {
		t.Fatalf("server on a new port rejected: %q", m2.err)
	}
}

// Octavia's server-side check stays authoritative for races: a 409 is
// reported as a possible duplicate, not as a generic error.
func TestConflictFromOctaviaIsReportedClearly(t *testing.T) {
	client, cleanup := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pools/pool-1/members") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"faultstring":"Another member on this pool is already using ip 10.0.0.9 on protocol_port 80"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer cleanup()

	m := New(client, nil, "pool-1", "web", "10.0.0.1", existingMembers)
	m.addressSource = addressSourceIP
	m.addrInput.SetValue("10.0.0.9")
	m.portInput.SetValue("80")
	m, cmd := m.submit()
	if cmd == nil {
		t.Fatalf("submit rejected: %s", m.err)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("submit did not return a batch")
	}
	m, _ = m.Update(batch[len(batch)-1]())
	if m.submitting || !strings.Contains(m.err, "address and port") {
		t.Fatalf("err = %q, want a clear duplicate/conflict message", m.err)
	}
}
