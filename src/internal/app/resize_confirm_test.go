package app

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
	"github.com/larkly/lazystack/internal/ui/serverlist"
)

type resizeFixture struct {
	log      *requestLog
	failIDs  map[string]bool
	statuses map[string]string
}

// resizeModel serves a server list with the given statuses and records
// resize confirm/revert requests.
func resizeModel(t *testing.T, st *resizeFixture) (Model, string) {
	t.Helper()
	m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/action"):
			id := strings.Split(r.URL.Path, "/")[2]
			st.log.add("POST " + id)
			if st.failIDs[id] {
				http.Error(w, "resize step failed", http.StatusConflict)
				return
			}
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "revertResize") {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/servers/detail":
			var parts []string
			for id, status := range st.statuses {
				parts = append(parts, fmt.Sprintf(`{"id":%q,"name":%q,"status":%q,"flavor":{"id":"f"}}`, id, id, status))
			}
			fmt.Fprintf(w, `{"servers":[%s]}`, strings.Join(parts, ","))
		case r.URL.Path == "/flavors/detail":
			fmt.Fprint(w, `{"flavors":[]}`)
		case r.URL.Path == "/images":
			fmt.Fprint(w, `{"images":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	m.serverList = serverlist.New(m.client.Compute, m.client.Image, time.Hour)
	loadViewData(m.serverList.Init(), func(msg tea.Msg) tea.Cmd { var cmd tea.Cmd; m.serverList, cmd = m.serverList.Update(msg); return cmd })
	m.view = viewServerList
	return m, path
}

func detailFor(id, status string) serverdetail.Model {
	d := serverdetail.New(nil, nil, nil, id, time.Hour)
	d.SetServer(&compute.Server{ID: id, Name: id, Status: status})
	return d
}

func TestResizeConfirmFailureRestoresVerifyResizeState(t *testing.T) {
	for _, revert := range []bool{false, true} {
		t.Run(fmt.Sprintf("revert=%v", revert), func(t *testing.T) {
			st := &resizeFixture{log: &requestLog{}, failIDs: map[string]bool{"a": true}}
			m, _ := resizeModel(t, st)
			m.view = viewServerDetail
			m.serverDetail = detailFor("a", "VERIFY_RESIZE")
			step := m.doConfirmResize
			pending := "Resize confirmed"
			if revert {
				step, pending = m.doRevertResize, "Resize reverted"
			}
			m, cmd := step()
			if cmd == nil || !strings.Contains(m.serverDetail.View(), pending) {
				t.Fatal("optimistic pending state not set")
			}
			m, _ = deliver(t, m, cmd)
			if m.activeModal != modalError {
				t.Fatal("failure not reported")
			}
			if !strings.Contains(m.serverDetail.Hints(), "^y confirm resize") {
				t.Errorf("VERIFY_RESIZE hints not restored: %q", m.serverDetail.Hints())
			}
			if strings.Contains(m.serverDetail.View(), pending) || strings.Contains(m.statusBar.Hint, "✓") {
				t.Errorf("success/waiting text left after failure: hint=%q", m.statusBar.Hint)
			}
		})
	}
}

func TestLateResizeFailureDoesNotResetOtherServer(t *testing.T) {
	st := &resizeFixture{log: &requestLog{}, failIDs: map[string]bool{"a": true}}
	m, _ := resizeModel(t, st)
	m.view = viewServerDetail
	m.serverDetail = detailFor("a", "VERIFY_RESIZE")
	m, cmdA := m.doConfirmResize()
	// The user moves to server B and confirms its resize before A's
	// request fails.
	m.serverDetail = detailFor("b", "VERIFY_RESIZE")
	m, cmdB := m.doConfirmResize()
	if cmdA == nil || cmdB == nil {
		t.Fatal("confirmations not scheduled")
	}
	m, _ = deliver(t, m, cmdA)
	if strings.Contains(m.serverDetail.Hints(), "^y confirm resize") || !strings.Contains(m.serverDetail.View(), "Resize confirmed") {
		t.Fatal("late failure for A reset B's pending state")
	}
}

func TestResizeConfirmEligibility(t *testing.T) {
	t.Run("bulk none eligible", func(t *testing.T) {
		st := &resizeFixture{log: &requestLog{}, statuses: map[string]string{"a": "ACTIVE", "b": "SHUTOFF"}}
		m, _ := resizeModel(t, st)
		m.serverList.SelectIDs([]string{"a", "b"})
		m, cmd := m.doConfirmResize()
		if cmd != nil || !strings.Contains(m.statusBar.StickyHint, "VERIFY_RESIZE") {
			t.Fatalf("cmd=%v hint=%q", cmd != nil, m.statusBar.StickyHint)
		}
		if m.serverList.SelectionCount() != 2 {
			t.Error("selection dropped although nothing ran")
		}
	})
	t.Run("single ineligible", func(t *testing.T) {
		st := &resizeFixture{log: &requestLog{}, statuses: map[string]string{"a": "ACTIVE"}}
		m, _ := resizeModel(t, st)
		for _, revert := range []bool{false, true} {
			step := m.doConfirmResize
			if revert {
				step = m.doRevertResize
			}
			next, cmd := step()
			if cmd != nil || !strings.Contains(next.statusBar.StickyHint, "not awaiting resize confirmation") {
				t.Fatalf("revert=%v cmd=%v hint=%q", revert, cmd != nil, next.statusBar.StickyHint)
			}
		}
		m.view = viewServerDetail
		m.serverDetail = detailFor("a", "ACTIVE")
		if _, cmd := m.doConfirmResize(); cmd != nil {
			t.Fatal("detail view sent confirm for ineligible server")
		}
	})
	t.Run("mixed selection", func(t *testing.T) {
		st := &resizeFixture{log: &requestLog{}, statuses: map[string]string{"a": "VERIFY_RESIZE", "b": "ACTIVE", "c": "VERIFY_RESIZE"}, failIDs: map[string]bool{"c": true}}
		m, path := resizeModel(t, st)
		m.serverList.SelectIDs([]string{"a", "b", "c"})
		m, cmd := m.doConfirmResize()
		msg := unwrapResult(runBounded(t, cmd))
		r, ok := msg.(bulkResultMsg)
		if !ok || len(r.succeeded) != 1 || len(r.failed) != 1 || len(r.skipped) != 1 || r.skipped[0].ref.ID != "b" {
			t.Fatalf("result=%#v", msg)
		}
		if s := r.summary(); !strings.Contains(s, "1 of 3 servers succeeded, 1 failed, 1 skipped") {
			t.Errorf("summary=%q", s)
		}
		if st.log.count("POST b") != 0 {
			t.Error("ineligible server was sent a confirm")
		}
		entries, _ := audit.ReadEntries(path, 10)
		byID := map[string]audit.Entry{}
		for _, e := range entries {
			byID[e.ResourceID] = e
		}
		for id, want := range map[string]string{"a": "success", "c": "error", "b": "skipped"} {
			if e := byID[id]; e.Action != audit.ActionConfirmResize || e.Result != want {
				t.Errorf("audit[%s]=%+v want %s", id, e, want)
			}
		}
	})
}

func TestSingleResizeStepIsAudited(t *testing.T) {
	for _, revert := range []bool{false, true} {
		st := &resizeFixture{log: &requestLog{}, statuses: map[string]string{"a": "VERIFY_RESIZE"}}
		m, path := resizeModel(t, st)
		step, want := m.doConfirmResize, audit.ActionConfirmResize
		if revert {
			step, want = m.doRevertResize, audit.ActionRevertResize
		}
		_, cmd := step()
		runBounded(t, cmd)
		checkAudit(t, path, want, "server", "a", "a", false)
	}
}

func TestResizeConfirmRejectedWhileDeletePending(t *testing.T) {
	st := &resizeFixture{log: &requestLog{}, statuses: map[string]string{"a": "VERIFY_RESIZE"}}
	m, _ := resizeModel(t, st)
	res, del := m.Update(confirmed("delete", "a"))
	m = res.(Model)
	if del == nil {
		t.Fatal("delete not scheduled")
	}
	m, cmd := m.doConfirmResize()
	if cmd != nil || !strings.Contains(m.statusBar.StickyHint, "in progress") {
		t.Fatalf("conflicting resize confirm scheduled: hint=%q", m.statusBar.StickyHint)
	}
	if st.log.count("POST a") != 0 {
		t.Fatal("conflicting request sent")
	}
}
