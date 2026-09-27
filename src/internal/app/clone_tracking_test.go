package app

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/ui/cloneprogress"
	"github.com/larkly/lazystack/internal/ui/servercreate"
)

func cloneFixture(t *testing.T) Model {
	t.Helper()
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"volumes":[],"servers":[]}`)
	})
	m.view = viewServerList
	return m
}

func startClone(t *testing.T, m Model, server string) Model {
	t.Helper()
	next, _ := m.Update(servercreate.ServerCloneCreatedMsg{
		Server:    &compute.Server{ID: server + "-id", Name: server},
		VolumeIDs: []string{server + "-src"},
	})
	return next.(Model)
}

// Dismissing clone A and starting clone B must not orphan A: its late
// messages still reach A, and its completion is still reported.
func TestDismissedCloneKeepsTrackingWhileAnotherStarts(t *testing.T) {
	m := cloneFixture(t)
	m = startClone(t, m, "alpha")
	a := m.cloneProgress
	// Name resolution for A completes only after B has started.
	resolveA := quickMessages(a.Init())
	next, _ := m.Update(press("esc"))
	m = next.(Model)
	if m.cloneProgress.Active {
		t.Fatal("setup: esc should dismiss the clone overlay")
	}
	m = startClone(t, m, "beta")
	b := m.cloneProgress
	if b.ID() == a.ID() {
		t.Fatal("setup: clones share an ID")
	}
	routedToA := false
	for _, msg := range resolveA {
		if _, ok := cloneprogress.OpID(msg); !ok {
			continue
		}
		next, cmd := m.Update(msg)
		m = next.(Model)
		if cmd != nil {
			routedToA = true
		}
	}
	if !routedToA {
		t.Fatal("A's late messages were not delivered to A")
	}
	if m.cloneProgress.ID() != b.ID() || !m.cloneProgress.Active {
		t.Fatal("A's messages replaced or hid clone B")
	}

	next, _ = m.Update(cloneprogress.AllCompleteMsg{Op: a.ID(), ServerName: "alpha"})
	m = next.(Model)
	if !strings.Contains(m.statusBar.StickyHint, "alpha") {
		t.Fatalf("A's completion not reported, hint = %q", m.statusBar.StickyHint)
	}
	if !m.cloneProgress.Active || m.cloneProgress.ID() != b.ID() {
		t.Fatal("A's completion closed clone B's overlay")
	}
	if len(m.cloneBackground) != 0 {
		t.Fatalf("finished clone A still tracked: %d", len(m.cloneBackground))
	}

	next, _ = m.Update(cloneprogress.RollbackCompleteMsg{Op: b.ID(), Cause: errTest, Leftover: []string{"volume beta-vol"}})
	m = next.(Model)
	if m.cloneProgress.Active || m.activeModal != modalError {
		t.Fatal("B's rollback should close its overlay and show the error")
	}
	if !strings.Contains(m.errModal.View(), "beta-vol") {
		t.Fatal("rollback error does not report the resources left behind")
	}
}
