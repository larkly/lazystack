package app

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/imageview"
	"github.com/larkly/lazystack/internal/ui/lbview"
	"github.com/larkly/lazystack/internal/ui/modal"
	"github.com/larkly/lazystack/internal/ui/volumelist"
)

func refs(ids ...string) []modal.ServerRef {
	out := make([]modal.ServerRef, len(ids))
	for i, id := range ids {
		out[i] = modal.ServerRef{ID: id, Name: "n-" + id}
	}
	return out
}

func hasRefresh(cmd tea.Cmd, want func(tea.Msg) bool) bool {
	for _, msg := range commandMessages(cmd) {
		if want(msg) {
			return true
		}
	}
	return false
}

func TestBulkServerPartialFailureReportsCountsAndRefreshes(t *testing.T) {
	m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/bad/") {
			http.Error(w, "cannot stop bad", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
	action := confirmed("stop", "")
	action.Servers = refs("s1", "s2", "bad", "s3", "s4")
	res, cmd := m.Update(action)
	m, next := deliver(t, res.(Model), cmd)

	if hint := m.statusBar.StickyHint; !strings.Contains(hint, "4 of 5 servers succeeded") || !strings.Contains(hint, "1 failed") {
		t.Fatalf("status=%q", hint)
	}
	if m.activeModal != modalError {
		t.Fatal("failure details not shown")
	}
	details := m.errModal.Context + "\n" + m.errModal.FriendlyError
	if !strings.Contains(details, "n-bad") || !strings.Contains(details, "cannot stop bad") || strings.Contains(details, "n-s1") {
		t.Errorf("failure details=%q", details)
	}
	if !hasRefresh(next, func(msg tea.Msg) bool { _, ok := msg.(shared.RefreshServersMsg); return ok }) {
		t.Error("server data not refreshed after partial success")
	}
	if got := m.serverList.SelectionCount(); got != 1 {
		t.Errorf("failed target not kept selected for retry: selection=%d", got)
	}
	entries, _ := audit.ReadEntries(path, 20)
	if len(entries) != 5 {
		t.Errorf("audit entries=%d, want one per server", len(entries))
	}
}

func TestResourceBulkAuditAndStructuredOutcome(t *testing.T) {
	for _, action := range []string{"delete_volumes_bulk", "delete_images_bulk", "detach_volumes_bulk"} {
		t.Run(action, func(t *testing.T) {
			m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if action == "detach_volumes_bulk" && r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/volumes/") {
					switch {
					case strings.HasSuffix(r.URL.Path, "/lookupfail"):
						http.Error(w, "lookup failed", http.StatusForbidden)
					case strings.HasSuffix(r.URL.Path, "/unattached"):
						fmt.Fprint(w, `{"volume":{"status":"available","attachments":[]}}`)
					default:
						fmt.Fprint(w, `{"volume":{"status":"in-use","attachments":[{"server_id":"srv"}]}}`)
					}
					return
				}
				if action == "detach_volumes_bulk" && r.Method == http.MethodGet {
					fmt.Fprint(w, `{"volumeAttachments":[]}`)
					return
				}
				if strings.Contains(r.URL.Path, "bad") {
					http.Error(w, "mutation failed", http.StatusConflict)
					return
				}
				w.WriteHeader(http.StatusAccepted)
			})
			m.volumeList = volumelist.New(m.client.BlockStorage, m.client.Compute, time.Hour)
			m.imageView = imageview.New(m.client.Image, m.client.Compute, time.Hour)
			targets := refs("good", "bad")
			if action == "detach_volumes_bulk" {
				targets = append(targets, refs("lookupfail", "unattached")...)
			}
			res, cmd := m.Update(modal.ConfirmAction{Action: action, Servers: targets, Confirm: true})
			msg := unwrapResult(runBounded(t, cmd))
			r, ok := msg.(bulkResultMsg)
			if !ok {
				t.Fatalf("result=%#v", msg)
			}
			wantFailed := []string{"bad"}
			if action == "detach_volumes_bulk" {
				wantFailed = append(wantFailed, "lookupfail")
			}
			if len(r.succeeded) != 1 || r.succeeded[0].ID != "good" || len(r.failed) != len(wantFailed) {
				t.Fatalf("outcome=%+v", r)
			}
			for i, id := range wantFailed {
				if r.failed[i].ref.ID != id || r.failed[i].err == nil {
					t.Errorf("failed[%d]=%+v want %s", i, r.failed[i], id)
				}
			}
			if action == "detach_volumes_bulk" && (len(r.skipped) != 1 || r.skipped[0].ref.ID != "unattached") {
				t.Errorf("skipped=%+v", r.skipped)
			}

			byID := map[string]audit.Entry{}
			entries, _ := audit.ReadEntries(path, 20)
			for _, e := range entries {
				byID[e.ResourceID] = e
			}
			wantResult := map[string]string{"good": "success", "bad": "error", "lookupfail": "error", "unattached": "skipped"}
			for id, result := range wantResult {
				if action != "detach_volumes_bulk" && (id == "lookupfail" || id == "unattached") {
					continue
				}
				e := byID[id]
				if e.Result != result || e.Cloud != "test-cloud" || e.Project != "test-project" || e.ResourceName != "n-"+id {
					t.Errorf("audit[%s]=%+v want %s", id, e, result)
				}
				if (result != "success") != (e.Error != "") {
					t.Errorf("audit[%s] error=%q", id, e.Error)
				}
			}

			m = res.(Model)
			m.view = viewVolumeList
			if action == "delete_images_bulk" {
				m.view = viewImageView
			}
			m, next := m.handleBulkResult(r)
			if !strings.Contains(m.statusBar.StickyHint, "1 failed") && action != "detach_volumes_bulk" {
				t.Errorf("status=%q", m.statusBar.StickyHint)
			}
			if next == nil {
				t.Error("no refresh after bulk mutation")
			}
			sel := m.volumeList.SelectionCount()
			if action == "delete_images_bulk" {
				sel = m.imageView.SelectionCount()
			}
			if sel != len(wantFailed) {
				t.Errorf("retryable selection=%d want %d", sel, len(wantFailed))
			}
		})
	}
}

type memberFixture struct {
	mu        sync.Mutex
	executing bool
	actions   []string
}

func lbMemberFixture(t *testing.T, st *memberFixture, mode string) (Model, string) {
	t.Helper()
	m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		st.mu.Lock()
		executing := st.executing
		if executing {
			st.actions = append(st.actions, r.Method+" "+r.URL.Path)
		}
		st.mu.Unlock()
		if r.Method == http.MethodDelete {
			if mode == "delete-error" && r.URL.Path == "/lbaas/pools/pool/members/b" {
				http.Error(w, "member failed", http.StatusConflict)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}
		switch r.URL.Path {
		case "/lbaas/loadbalancers":
			fmt.Fprint(w, `{"loadbalancers":[{"id":"lb","name":"fixture-lb","provisioning_status":"ACTIVE"}]}`)
		case "/lbaas/loadbalancers/lb":
			if executing && mode == "wait-error" {
				http.Error(w, "gone", http.StatusNotFound)
				return
			}
			fmt.Fprint(w, `{"loadbalancer":{"id":"lb","provisioning_status":"ACTIVE"}}`)
		case "/lbaas/listeners":
			fmt.Fprint(w, `{"listeners":[]}`)
		case "/lbaas/pools":
			fmt.Fprint(w, `{"pools":[{"id":"pool","name":"fixture-pool"}]}`)
		case "/lbaas/pools/pool/members":
			fmt.Fprint(w, `{"members":[{"id":"a","name":"alpha"},{"id":"b","name":"beta"},{"id":"c","name":"gamma"}]}`)
		default:
			t.Errorf("unexpected %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	m.lbView = lbview.New(m.client.LoadBalancer, m.refreshInterval)
	loadViewData(m.lbView.Init(), func(msg tea.Msg) tea.Cmd { var cmd tea.Cmd; m.lbView, cmd = m.lbView.Update(msg); return cmd })
	m.view = viewLBView
	return m, path
}

// The confirmation must carry the member targets it showed: a refresh or
// cleared selection before confirming must not turn it into a clean
// zero-member delete.
func TestLBBulkMemberDeleteUsesCapturedTargets(t *testing.T) {
	for _, mode := range []string{"success", "delete-error", "wait-error"} {
		t.Run(mode, func(t *testing.T) {
			st := &memberFixture{}
			m, path := lbMemberFixture(t, st, mode)
			m.lbView.ToggleAllMemberSelection()
			m, _ = m.openLBBulkMemberDeleteConfirm()
			if m.activeModal != modalConfirm {
				t.Fatal("no confirmation")
			}
			action := confirmCurrent(m)
			m.lbView.ClearMemberSelection() // selection lost while the dialog was open
			st.mu.Lock()
			st.executing = true
			st.mu.Unlock()
			res, cmd := m.Update(action)
			m = res.(Model)
			msg := unwrapResult(runBounded(t, cmd))
			r, ok := msg.(bulkResultMsg)
			if !ok {
				t.Fatalf("result=%#v", msg)
			}
			total := len(r.succeeded) + len(r.failed) + len(r.skipped)
			if total != 3 {
				t.Fatalf("targets accounted=%d, want 3 (%+v)", total, r)
			}
			st.mu.Lock()
			deletes := 0
			for _, a := range st.actions {
				if strings.HasPrefix(a, "DELETE ") {
					deletes++
				}
			}
			st.mu.Unlock()
			switch mode {
			case "success":
				if deletes != 3 || len(r.succeeded) != 3 {
					t.Fatalf("deletes=%d outcome=%+v", deletes, r)
				}
			case "delete-error":
				if len(r.failed) != 1 || r.failed[0].ref.ID != "b" || r.failed[0].ref.Name != "beta" {
					t.Fatalf("failed=%+v", r.failed)
				}
			case "wait-error":
				// a is deleted, then waiting for the LB fails: b and c are
				// reported as not attempted, by ID and name.
				if deletes != 1 || len(r.failed) != 2 || r.failed[0].ref.ID != "b" || r.failed[1].ref.Name != "gamma" {
					t.Fatalf("deletes=%d failed=%+v", deletes, r.failed)
				}
			}
			entries, _ := audit.ReadEntries(path, 20)
			if len(entries) != 3 {
				t.Errorf("audit entries=%d, want one per member", len(entries))
			}
			for _, e := range entries {
				if e.ResourceType != "lb_member" || e.Action != audit.ActionDeleteLB {
					t.Errorf("audit=%+v", e)
				}
			}
		})
	}
}
