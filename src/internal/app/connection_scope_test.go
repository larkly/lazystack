package app

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/larkly/lazystack/internal/shared"
)

// cloudFixture is one fake cloud/project: a compute endpoint listing the
// given servers and an identity endpoint listing the given projects.
type cloudFixture struct {
	client *gophercloud.ServiceClient
	pc     *gophercloud.ProviderClient
}

func newCloudFixture(t *testing.T, servers, projects string, projectStatus int) cloudFixture {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/servers/detail":
			fmt.Fprintf(w, `{"servers":%s}`, servers)
		case strings.HasSuffix(r.URL.Path, "/auth/projects"):
			w.WriteHeader(projectStatus)
			fmt.Fprintf(w, `{"projects":%s,"links":{}}`, projects)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	t.Cleanup(srv.Close)
	pc := &gophercloud.ProviderClient{IdentityBase: srv.URL + "/"}
	return cloudFixture{
		client: &gophercloud.ServiceClient{ProviderClient: pc, Endpoint: srv.URL + "/"},
		pc:     pc,
	}
}

// withTokenProject makes the provider's auth token scoped to a project.
func (f cloudFixture) withTokenProject(id, name string) cloudFixture {
	res := tokens.CreateResult{}
	res.Body = map[string]any{"token": map[string]any{"project": map[string]any{"id": id, "name": name}}}
	res.Header = http.Header{"X-Subject-Token": {"token-" + id}}
	_ = f.pc.SetTokenAndAuthResult(res)
	return f
}

func (f cloudFixture) connected() shared.CloudConnectedMsg {
	return shared.CloudConnectedMsg{
		ComputeClient: f.client, ImageClient: f.client, NetworkClient: f.client,
		ProviderClient: f.pc,
	}
}

// unwrapScoped returns the message inside a connection-scope envelope.
func unwrapScoped(msg tea.Msg) tea.Msg {
	if scoped, ok := msg.(connScopedMsg); ok {
		return scoped.msg
	}
	return msg
}

func isRefreshTick(msg tea.Msg) bool {
	name := fmt.Sprintf("%T", unwrapScoped(msg))
	return strings.HasSuffix(name, "TickMsg") && !strings.Contains(name, "spinner")
}

// errTest is a generic error for result messages in tests.
var errTest = errors.New("test error")

func deliverMsgs(m Model, msgs []tea.Msg) (Model, []tea.Msg) {
	var out []tea.Msg
	for _, msg := range msgs {
		next, cmd := m.Update(msg)
		m = next.(Model)
		out = append(out, quickMessages(cmd)...)
	}
	return m, out
}

func TestOldConnectionListResultIsDiscarded(t *testing.T) {
	oldCloud := newCloudFixture(t, `[{"id":"old-vm","name":"old-project-vm","status":"ACTIVE"}]`, `[]`, 200)
	newCloud := newCloudFixture(t, `[]`, `[]`, 200)
	m := initTestModel()
	m.refreshInterval = time.Hour
	next, cmdOld := m.Update(oldCloud.connected())
	m = next.(Model)
	heldOld := quickMessages(cmdOld) // old project's list response, held back
	next, cmdNew := m.Update(newCloud.connected())
	m = next.(Model)
	m, _ = deliverMsgs(m, quickMessages(cmdNew))
	m, _ = deliverMsgs(m, heldOld)
	if s := m.serverList.SelectedServer(); s != nil {
		t.Fatalf("old project's server %q leaked into the new connection", s.Name)
	}
}

func TestOldTicksDoNotChainAfterReconnectOrResume(t *testing.T) {
	a := newCloudFixture(t, `[]`, `[]`, 200)
	b := newCloudFixture(t, `[]`, `[]`, 200)
	m := initTestModel()
	m.refreshInterval = time.Millisecond
	next, cmdA := m.Update(a.connected())
	m = next.(Model)
	oldTicks := filterTicks(quickMessages(cmdA))
	if len(oldTicks) != 1 {
		t.Fatalf("setup: connect scheduled %d ticks", len(oldTicks))
	}
	next, cmdB := m.Update(b.connected())
	m = next.(Model)
	liveTicks := filterTicks(quickMessages(cmdB))
	if len(liveTicks) != 1 {
		t.Fatalf("setup: reconnect scheduled %d ticks", len(liveTicks))
	}

	m, out := deliverMsgs(m, oldTicks)
	if n := len(filterTicks(out)); n != 0 {
		t.Fatalf("old connection's tick chained %d more ticks", n)
	}
	m, out = deliverMsgs(m, liveTicks)
	if n := len(filterTicks(out)); n != 1 {
		t.Fatalf("live tick chained %d ticks, want 1", n)
	}
	staleAfterResume := filterTicks(out)

	// Idle pause and resume: the resume starts a fresh chain, so a tick
	// from before the pause must be ignored.
	m.idlePaused = true
	next, cmd := m.Update(press("x"))
	m = next.(Model)
	if n := len(filterTicks(quickMessages(cmd))); n != 1 {
		t.Fatalf("resume started %d chains", n)
	}
	_, out = deliverMsgs(m, staleAfterResume)
	if n := len(filterTicks(out)); n != 0 {
		t.Fatalf("pre-pause tick chained %d ticks after resume", n)
	}
}

func filterTicks(msgs []tea.Msg) []tea.Msg {
	var out []tea.Msg
	for _, msg := range msgs {
		if isRefreshTick(msg) {
			out = append(out, msg)
		}
	}
	return out
}

func TestCloudSwitchResetsProjectIdentityFromToken(t *testing.T) {
	c := newCloudFixture(t, `[]`, `[]`, http.StatusInternalServerError).withTokenProject("new-id", "New Project")
	m := initTestModel()
	m.projects = []shared.ProjectInfo{{ID: "old", Name: "Old"}, {ID: "o2", Name: "O2"}}
	m.currentProjectID = "old"
	m.statusBar.ProjectName = "Old"
	next, cmd := m.Update(c.connected())
	m = next.(Model)
	if m.currentProjectID != "new-id" || m.statusBar.ProjectName != "New Project" {
		t.Fatalf("identity after connect = %q/%q, want the token's project", m.currentProjectID, m.statusBar.ProjectName)
	}
	if len(m.projects) != 0 {
		t.Fatalf("old project list survived the switch: %v", m.projects)
	}
	// The project list fails: surface it without losing the token scope.
	m, _ = deliverMsgs(m, quickMessages(cmd))
	if m.currentProjectID != "new-id" {
		t.Fatalf("project list failure changed scope to %q", m.currentProjectID)
	}
	if !strings.Contains(strings.ToLower(m.statusBar.StickyHint), "project") || m.activeModal != modalNone {
		t.Fatalf("project list failure not surfaced non-fatally: hint=%q modal=%v", m.statusBar.StickyHint, m.activeModal)
	}
}

func TestFailedProjectSwitchKeepsPriorIdentity(t *testing.T) {
	m := initTestModel()
	m.cloudName = "c1"
	m.projects = []shared.ProjectInfo{{ID: "old", Name: "Old"}, {ID: "p2", Name: "P2"}}
	m.currentProjectID = "old"
	m.statusBar.ProjectName = "Old"
	next, _ := m.Update(shared.ProjectSelectedMsg{ProjectID: "p2", ProjectName: "P2"})
	m = next.(Model)
	if m.currentProjectID != "old" || m.statusBar.ProjectName != "Old" {
		t.Fatalf("identity switched before the connection succeeded: %q/%q", m.currentProjectID, m.statusBar.ProjectName)
	}
	next, _ = m.Update(shared.CloudConnectErrMsg{Err: errTest})
	m = next.(Model)
	if m.currentProjectID != "old" || m.cloudName != "c1" || m.activeModal != modalError {
		t.Fatal("failed switch must keep the prior identity and report the error")
	}
}

func TestStaleProjectListIsDiscarded(t *testing.T) {
	a := newCloudFixture(t, `[]`, `[{"id":"a1","name":"A1","enabled":true},{"id":"a2","name":"A2","enabled":true}]`, 200)
	b := newCloudFixture(t, `[]`, `[{"id":"b1","name":"B1","enabled":true}]`, 200).withTokenProject("b1", "B1")
	m := initTestModel()
	m.refreshInterval = time.Hour
	next, cmdA := m.Update(a.connected())
	m = next.(Model)
	heldA := quickMessages(cmdA)
	next, cmdB := m.Update(b.connected())
	m = next.(Model)
	m, _ = deliverMsgs(m, quickMessages(cmdB))
	m, _ = deliverMsgs(m, heldA)
	for _, p := range m.projects {
		if strings.HasPrefix(p.ID, "a") {
			t.Fatalf("stale project list from the previous cloud applied: %v", m.projects)
		}
	}
	if m.currentProjectID != "b1" {
		t.Fatalf("current project = %q, want b1", m.currentProjectID)
	}
}

func TestOldConnectionResultsCannotAlterNewUI(t *testing.T) {
	c := newCloudFixture(t, `[]`, `[]`, 200)
	for name, msg := range map[string]tea.Msg{
		"server action error":   shared.ServerActionErrMsg{Action: "Delete", Name: "old-vm", Err: errTest},
		"resource action error": shared.ResourceActionErrMsg{Action: "Delete", Name: "old-vol", Err: errTest},
		"server action":         shared.ServerActionMsg{Action: "Reboot", Name: "old-vm"},
		"project list":          shared.ProjectsLoadedMsg{Projects: []shared.ProjectInfo{{ID: "x", Name: "X"}}, CurrentID: "x"},
		"project list error":    projectsLoadErrMsg{err: errTest},
	} {
		t.Run(name, func(t *testing.T) {
			m := initTestModel()
			next, _ := m.Update(c.connected())
			m = next.(Model)
			stale := connScopedMsg{gen: m.connGen - 1, msg: msg}
			hint, view := m.statusBar.StickyHint, m.view
			next, cmd := m.Update(stale)
			m = next.(Model)
			if cmd != nil || m.activeModal != modalNone || m.statusBar.StickyHint != hint || m.view != view || len(m.projects) != 0 {
				t.Fatal("stale result changed the new connection's UI")
			}
			// The same result for the current connection is applied.
			next, _ = m.Update(connScopedMsg{gen: m.connGen, msg: msg})
			m = next.(Model)
			if m.activeModal == modalNone && m.statusBar.StickyHint == hint && len(m.projects) == 0 && m.statusBar.Error == "" {
				t.Fatal("current-connection result was not applied")
			}
		})
	}
}

func TestOnlyLatestConnectAttemptIsApplied(t *testing.T) {
	a := newCloudFixture(t, `[]`, `[]`, 200)
	b := newCloudFixture(t, `[]`, `[]`, 200)
	m := initTestModel()
	next, _ := m.Update(shared.CloudSelectedMsg{CloudName: "alpha"})
	m = next.(Model)
	first := m.connectSeq
	next, _ = m.Update(shared.CloudSelectedMsg{CloudName: "beta"})
	m = next.(Model)
	if m.cloudName != "" {
		t.Fatalf("cloud name committed before connecting: %q", m.cloudName)
	}
	next, _ = m.Update(connectResultMsg{seq: m.connectSeq, cloudName: "beta", msg: b.connected()})
	m = next.(Model)
	next, _ = m.Update(connectResultMsg{seq: first, cloudName: "alpha", msg: a.connected()})
	m = next.(Model)
	if m.cloudName != "beta" || m.client.Compute != b.client {
		t.Fatalf("late result of an older attempt replaced the connection: cloud=%q", m.cloudName)
	}
}
