package lblistenercreate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/testutil"
)

var lbPools = []loadbalancer.Pool{
	{ID: "pool-http", Name: "web", Protocol: "HTTP"},
	{ID: "pool-udp", Name: "dns", Protocol: "UDP"},
}

// fakeListeners records listener create/update bodies. echoPool controls
// whether the response echoes the requested default pool.
type fakeListeners struct {
	mu       sync.Mutex
	echoPool bool
	bodies   []map[string]any
}

func (f *fakeListeners) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body struct {
		Listener map[string]any `json:"listener"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.bodies = append(f.bodies, body.Listener)
	pool := ""
	if p, ok := body.Listener["default_pool_id"].(string); ok && f.echoPool {
		pool = p
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/listeners"):
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/listeners/lst-1"):
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		return
	}
	fmt.Fprintf(w, `{"listener":{"id":"lst-1","protocol":"HTTP","protocol_port":80,"default_pool_id":%q}}`, pool)
}

func runSubmit(t *testing.T, m Model) (Model, tea.Msg) {
	t.Helper()
	m, cmd := m.submit()
	if cmd == nil {
		t.Fatalf("submit rejected: %s", m.err)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("submit did not return a batch")
	}
	msg := batch[len(batch)-1]()
	m, _ = m.Update(msg)
	return m, msg
}

func focusPool(m Model, presses int) Model {
	m.focusField = fieldDefaultPool
	m.updateFocus()
	for i := 0; i < presses; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	}
	return m
}

func TestCreateListenerSendsDefaultPool(t *testing.T) {
	f := &fakeListeners{echoPool: true}
	client, cleanup := testutil.FakeServiceClient(f)
	defer cleanup()

	m := New(client, "lb-1", "edge", lbPools)
	m.nameInput.SetValue("front")
	m.selectedProtocol = 1 // HTTP
	m.portInput.SetValue("80")
	m = focusPool(m, 1)
	if !strings.Contains(m.View(), "web (HTTP)") {
		t.Fatalf("chosen pool not shown:\n%s", m.View())
	}

	m, msg := runSubmit(t, m)
	if _, ok := msg.(listenerCreatedMsg); !ok {
		t.Fatalf("result = %#v, want listenerCreatedMsg (err %q)", msg, m.err)
	}
	if len(f.bodies) != 1 || f.bodies[0]["default_pool_id"] != "pool-http" {
		t.Fatalf("POST bodies = %v, want default_pool_id pool-http", f.bodies)
	}
}

func TestCreateListenerWithoutPoolOmitsBinding(t *testing.T) {
	f := &fakeListeners{echoPool: true}
	client, cleanup := testutil.FakeServiceClient(f)
	defer cleanup()

	m := New(client, "lb-1", "edge", lbPools)
	m.nameInput.SetValue("front")
	m.portInput.SetValue("80")
	runSubmit(t, m)
	if _, ok := f.bodies[0]["default_pool_id"]; ok {
		t.Fatalf("default_pool_id sent without a choice: %v", f.bodies[0])
	}
}

func TestCreateListenerRejectsIncompatiblePool(t *testing.T) {
	m := New(nil, "lb-1", "edge", lbPools)
	m.nameInput.SetValue("front")
	m.selectedProtocol = 1 // HTTP
	m.portInput.SetValue("80")
	m = focusPool(m, 2) // UDP pool
	m, cmd := m.submit()
	if cmd != nil || !strings.Contains(m.err, "cannot use") {
		t.Fatalf("cmd=%v err=%q, want protocol mismatch before any request", cmd != nil, m.err)
	}
}

func TestCreateListenerFailsWhenBindingNotConfirmed(t *testing.T) {
	f := &fakeListeners{echoPool: false}
	client, cleanup := testutil.FakeServiceClient(f)
	defer cleanup()

	m := New(client, "lb-1", "edge", lbPools)
	m.nameInput.SetValue("front")
	m.selectedProtocol = 1
	m.portInput.SetValue("80")
	m = focusPool(m, 1)
	m, msg := runSubmit(t, m)
	if _, ok := msg.(listenerCreateErrMsg); !ok || m.Active == false {
		t.Fatalf("result = %#v, want an error while the binding is unconfirmed", msg)
	}
}

func TestEditListenerShowsAndChangesBinding(t *testing.T) {
	f := &fakeListeners{echoPool: true}
	client, cleanup := testutil.FakeServiceClient(f)
	defer cleanup()

	m := NewEdit(client, "lst-1", "front", "", 0, "edge", "HTTP", "pool-http", lbPools)
	if !strings.Contains(m.View(), "web (HTTP)") {
		t.Fatalf("current binding not shown:\n%s", m.View())
	}

	// Untouched: the binding is left alone.
	runSubmit(t, m)
	if _, ok := f.bodies[0]["default_pool_id"]; ok {
		t.Fatalf("untouched edit sent default_pool_id: %v", f.bodies[0])
	}

	// Cycle web -> dns -> none and submit: the binding is removed.
	m = focusPool(m, 2)
	runSubmit(t, m)
	v, ok := f.bodies[1]["default_pool_id"]
	if !ok || v != nil {
		t.Fatalf("PUT body = %v, want default_pool_id null", f.bodies[1])
	}
}
