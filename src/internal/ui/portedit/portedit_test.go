package portedit

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

// putRecorder is a fake Neutron port endpoint that records PUT bodies and,
// like Neutron, rejects list attributes sent as null.
func putRecorder(t *testing.T, bodies *[]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		*bodies = append(*bodies, string(raw))
		var body map[string]map[string]any
		_ = json.Unmarshal(raw, &body)
		for k, v := range body["port"] {
			if v == nil {
				http.Error(w, `{"NeutronError":{"message":"Invalid input for `+k+`"}}`, http.StatusBadRequest)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"port":{"id":"port-1"}}`))
	}
}

// run executes a submit command batch and returns the first non-spinner
// message.
func run(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			switch r := c().(type) {
			case portUpdatedMsg, portUpdateErrMsg:
				return r
			}
		}
		return nil
	}
	return msg
}

func loadedModel(t *testing.T, bodies *[]string) Model {
	client, cleanup := testutil.FakeServiceClient(putRecorder(t, bodies))
	t.Cleanup(cleanup)
	m := New(client, network.Port{ID: "port-1", Name: "web", AdminStateUp: true, PortSecurityEnabled: true, SecurityGroups: []string{"sg-1", "sg-2"}})
	m, _ = m.Update(sgLoadedMsg{sgs: []network.SecurityGroup{{ID: "sg-1", Name: "default"}, {ID: "sg-2", Name: "web"}, {ID: "sg-3", Name: "db"}}})
	return m
}

func TestClearingSecurityGroupsSendsEmptyArray(t *testing.T) {
	var bodies []string
	m := loadedModel(t, &bodies)
	m.selectedSGs = map[int]bool{}
	m, cmd := m.submit()
	msg := run(cmd)
	if len(bodies) != 1 || !strings.Contains(bodies[0], `"security_groups":[]`) {
		t.Fatalf("PUT bodies = %q, want security_groups:[]", bodies)
	}
	done, ok := msg.(portUpdatedMsg)
	if !ok {
		t.Fatalf("result = %#v, want portUpdatedMsg", msg)
	}
	m, cmd = m.Update(done)
	if action := cmd().(shared.ResourceActionMsg); action.Action != "Updated port" || m.Active {
		t.Fatalf("action = %+v active=%v", action, m.Active)
	}
}

func TestUnchangedSecurityGroupsOmitted(t *testing.T) {
	var bodies []string
	m := loadedModel(t, &bodies)
	m.nameInput.SetValue("web-renamed")
	_, cmd := m.submit()
	if _, ok := run(cmd).(portUpdatedMsg); !ok {
		t.Fatal("update failed")
	}
	if len(bodies) != 1 || strings.Contains(bodies[0], "security_groups") {
		t.Fatalf("PUT bodies = %q, security_groups should be omitted", bodies)
	}
}

func TestSecurityGroupLoadFailureNeverClearsGroups(t *testing.T) {
	var bodies []string
	client, cleanup := testutil.FakeServiceClient(putRecorder(t, &bodies))
	defer cleanup()
	for _, failed := range []bool{false, true} {
		bodies = nil
		m := New(client, network.Port{ID: "port-1", Name: "web", AdminStateUp: true, PortSecurityEnabled: true, SecurityGroups: []string{"sg-1"}})
		if failed {
			m, _ = m.Update(sgLoadErrMsg{err: errors.New("forbidden")})
		}
		m.nameInput.SetValue("renamed")
		_, cmd := m.submit()
		run(cmd)
		for _, b := range bodies {
			if strings.Contains(b, "security_groups") {
				t.Fatalf("failed=%v: security groups sent without a successful load: %s", failed, b)
			}
		}
	}
}
