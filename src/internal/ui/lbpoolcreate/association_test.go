package lbpoolcreate

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

var lbListeners = []loadbalancer.Listener{
	{ID: "lst-bound", Name: "legacy", Protocol: "HTTP", ProtocolPort: 8080, DefaultPoolID: "pool-old"},
	{ID: "lst-1", Name: "front", Protocol: "HTTP", ProtocolPort: 80},
}

type fakeOctavia struct {
	mu          sync.Mutex
	putStatus   int
	poolPosts   int
	listenerPut []map[string]any
}

func (f *fakeOctavia) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/pools"):
		f.poolPosts++
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"pool":{"id":"pool-new","name":"web","protocol":"HTTP","lb_algorithm":"ROUND_ROBIN"}}`)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/loadbalancers/lb-1"):
		fmt.Fprint(w, `{"loadbalancer":{"id":"lb-1","provisioning_status":"ACTIVE"}}`)
	case r.Method == http.MethodPut && strings.HasSuffix(p, "/listeners/lst-1"):
		var body struct {
			Listener map[string]any `json:"listener"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.listenerPut = append(f.listenerPut, body.Listener)
		if f.putStatus != 0 {
			http.Error(w, `{"faultstring":"Load Balancer lb-1 is immutable"}`, f.putStatus)
			return
		}
		fmt.Fprintf(w, `{"listener":{"id":"lst-1","default_pool_id":%q}}`, body.Listener["default_pool_id"])
	default:
		http.Error(w, "unexpected "+r.Method+" "+p, http.StatusNotFound)
	}
}

func submitPool(t *testing.T, m Model) (Model, tea.Msg) {
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

func httpPoolForm(listeners []loadbalancer.Listener) Model {
	m := New(nil, "lb-1", "edge", listeners)
	m.nameInput.SetValue("web")
	m.selectedProtocol = 1 // HTTP
	return m
}

func TestPoolFormOffersOnlyUnboundListeners(t *testing.T) {
	m := httpPoolForm(lbListeners)
	if len(m.listeners) != 1 || m.listeners[0].ID != "lst-1" {
		t.Fatalf("offered listeners = %+v, want only the unbound one", m.listeners)
	}
}

func TestCreatePoolBindsChosenListener(t *testing.T) {
	f := &fakeOctavia{}
	client, cleanup := testutil.FakeServiceClient(f)
	defer cleanup()

	m := httpPoolForm(lbListeners)
	m.client = client
	m.focusField = fieldListener
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if !strings.Contains(m.View(), "front (HTTP:80)") {
		t.Fatalf("chosen listener not shown:\n%s", m.View())
	}

	m, msg := submitPool(t, m)
	created, ok := msg.(poolCreatedMsg)
	if !ok || created.listener == "" {
		t.Fatalf("result = %#v (err %q), want a confirmed binding", msg, m.err)
	}
	if len(f.listenerPut) != 1 || f.listenerPut[0]["default_pool_id"] != "pool-new" {
		t.Fatalf("listener updates = %v, want default_pool_id pool-new", f.listenerPut)
	}
}

func TestCreatePoolWithoutListenerDoesNotTouchListeners(t *testing.T) {
	f := &fakeOctavia{}
	client, cleanup := testutil.FakeServiceClient(f)
	defer cleanup()

	m := httpPoolForm(lbListeners)
	m.client = client
	if _, msg := submitPool(t, m); msg != (poolCreatedMsg{}) {
		t.Fatalf("result = %#v", msg)
	}
	if len(f.listenerPut) != 0 {
		t.Fatalf("listener updated without a choice: %v", f.listenerPut)
	}
}

func TestCreatePoolRejectsIncompatibleListenerBeforeCreate(t *testing.T) {
	f := &fakeOctavia{}
	client, cleanup := testutil.FakeServiceClient(f)
	defer cleanup()

	m := httpPoolForm(lbListeners)
	m.client = client
	m.selectedProtocol = 3 // UDP
	m.selectedListener = 1
	m, cmd := m.submit()
	if cmd != nil || !strings.Contains(m.err, "cannot be the default pool") {
		t.Fatalf("cmd=%v err=%q, want protocol mismatch", cmd != nil, m.err)
	}
	if f.poolPosts != 0 {
		t.Fatal("pool created despite protocol mismatch")
	}
}

func TestCreatePoolReportsFailedBindingExplicitly(t *testing.T) {
	f := &fakeOctavia{putStatus: http.StatusConflict}
	client, cleanup := testutil.FakeServiceClient(f)
	defer cleanup()

	m := httpPoolForm(lbListeners)
	m.client = client
	m.selectedListener = 1
	m, msg := submitPool(t, m)
	if _, ok := msg.(poolCreateErrMsg); !ok {
		t.Fatalf("result = %#v, want an error", msg)
	}
	if !m.Active || !strings.Contains(m.err, "pool-new was created but is not attached to listener front") || !strings.Contains(m.err, "busy") {
		t.Fatalf("err = %q, want the partial result and the busy state", m.err)
	}
}

func TestPoolEditShowsCurrentBinding(t *testing.T) {
	m := NewEdit(nil, "pool-old", "old", "ROUND_ROBIN", "edge", lbListeners)
	if view := m.View(); !strings.Contains(view, "legacy (HTTP:8080)") {
		t.Fatalf("binding not shown:\n%s", view)
	}
}
