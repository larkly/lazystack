package portcreate

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/testutil"
)

// createRecorder records port create bodies.
func createRecorder(t *testing.T, bodies *[]map[string]any, fail bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]map[string]any
		_ = json.Unmarshal(raw, &body)
		*bodies = append(*bodies, body["port"])
		if fail {
			http.Error(w, `{"NeutronError":{"message":"quota exceeded"}}`, http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"port":{"id":"new-port"}}`))
	}
}

func runCreate(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			switch r := c().(type) {
			case portCreatedMsg, portCreateErrMsg:
				return r
			}
		}
	}
	return nil
}

var testSGs = []network.SecurityGroup{{ID: "sg-web", Name: "web"}, {ID: "sg-default", Name: "default"}}

func newCreate(t *testing.T, bodies *[]map[string]any, fail bool) Model {
	client, cleanup := testutil.FakeServiceClient(createRecorder(t, bodies, fail))
	t.Cleanup(cleanup)
	return New(client, "net-1", "app-net", []network.Subnet{{ID: "sub-1", Name: "app", CIDR: "10.0.0.0/24", IPVersion: 4}})
}

func TestSubmitWhileSecurityGroupsLoadingIsBlocked(t *testing.T) {
	var bodies []map[string]any
	m := newCreate(t, &bodies, false)
	m, cmd := m.submit()
	runCreate(cmd)
	if len(bodies) != 0 || m.submitting || m.err == "" {
		t.Fatalf("submitted while loading: bodies=%v err=%q", bodies, m.err)
	}
}

func TestSecurityGroupLoadFailureOmitsGroupsInsteadOfClearing(t *testing.T) {
	var bodies []map[string]any
	m := newCreate(t, &bodies, false)
	m, _ = m.Update(sgLoadErrMsg{err: errors.New("forbidden")})
	if !strings.Contains(m.View(), "forbidden") {
		t.Fatalf("load failure not shown:\n%s", m.View())
	}
	m, cmd := m.submit()
	if _, ok := runCreate(cmd).(portCreatedMsg); !ok {
		t.Fatalf("create did not run, err=%q", m.err)
	}
	if _, present := bodies[0]["security_groups"]; present {
		t.Fatalf("security_groups sent after failed discovery: %v", bodies[0])
	}
}

func TestSecurityGroupSelectionAfterSuccessfulLoad(t *testing.T) {
	var bodies []map[string]any
	m := newCreate(t, &bodies, false)
	m, _ = m.Update(sgLoadedMsg{sgs: testSGs})
	_, cmd := m.submit()
	runCreate(cmd)
	if got, _ := json.Marshal(bodies[0]["security_groups"]); string(got) != `["sg-default"]` {
		t.Fatalf("default not selected: %s", got)
	}

	// Explicitly deselecting everything still sends [].
	bodies = nil
	m = newCreate(t, &bodies, false)
	m, _ = m.Update(sgLoadedMsg{sgs: testSGs})
	m.selectedSGs = map[int]bool{}
	_, cmd = m.submit()
	runCreate(cmd)
	if got, _ := json.Marshal(bodies[0]["security_groups"]); string(got) != `[]` {
		t.Fatalf("explicit none = %s, want []", got)
	}
}

func TestCreateFailureKeepsSecurityGroupContext(t *testing.T) {
	var bodies []map[string]any
	m := newCreate(t, &bodies, true)
	m, _ = m.Update(sgLoadErrMsg{err: errors.New("forbidden")})
	m, cmd := m.submit()
	msg := runCreate(cmd)
	m, _ = m.Update(msg)
	view := m.View()
	if !strings.Contains(view, "quota exceeded") || !strings.Contains(view, "server default") {
		t.Fatalf("create failure lost context:\n%s", view)
	}
}
