package app

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/ui/modal"
)

type requestLog struct {
	mu   sync.Mutex
	reqs []string
}

func (l *requestLog) add(s string) {
	l.mu.Lock()
	l.reqs = append(l.reqs, s)
	l.mu.Unlock()
}

func (l *requestLog) count(s string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, r := range l.reqs {
		if r == s {
			n++
		}
	}
	return n
}

func confirmed(action, id string) modal.ConfirmAction {
	return modal.ConfirmAction{Action: action, ServerID: id, Name: id, Confirm: true}
}

// unwrapResult strips the in-flight tracking wrapper from a command result.
func unwrapResult(msg tea.Msg) tea.Msg {
	if r, ok := msg.(actionResultMsg); ok {
		return r.msg
	}
	return msg
}

// deliver feeds a command's result back into Update, as the runtime would.
func deliver(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	res, next := m.Update(runBounded(t, cmd))
	return res.(Model), next
}

func TestConfirmedMutationBlocksDuplicateUntilComplete(t *testing.T) {
	log := &requestLog{}
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		log.add(r.Method + " " + r.URL.Path)
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	res, first := m.Update(confirmed("delete", "srv-a"))
	m = res.(Model)
	if first == nil {
		t.Fatal("first delete not scheduled")
	}
	// The same confirmation again while the first request is still pending.
	res, dup := m.Update(confirmed("delete", "srv-a"))
	m = res.(Model)
	if dup != nil {
		t.Fatal("duplicate delete of an in-flight server was scheduled")
	}
	if !strings.Contains(m.statusBar.StickyHint, "in progress") {
		t.Errorf("no in-progress feedback: %q", m.statusBar.StickyHint)
	}
	// A conflicting action on the same server is rejected too...
	res, conflict := m.Update(confirmed("soft reboot", "srv-a"))
	m = res.(Model)
	if conflict != nil {
		t.Fatal("conflicting reboot of an in-flight server was scheduled")
	}
	// ...while an unrelated server is unaffected.
	res, other := m.Update(confirmed("soft reboot", "srv-b"))
	m = res.(Model)
	if other == nil {
		t.Fatal("unrelated server action was blocked")
	}

	m, _ = deliver(t, m, first)
	if n := log.count("DELETE /servers/srv-a"); n != 1 {
		t.Fatalf("delete requests=%d, want 1", n)
	}
	// Completion releases the lock.
	res, again := m.Update(confirmed("soft reboot", "srv-a"))
	if again == nil {
		t.Fatal("lock not released after completion")
	}
	m = res.(Model)
	m, _ = deliver(t, m, again)
	m, _ = deliver(t, m, other)
	if len(m.actions.inflight) != 0 {
		t.Fatalf("locks leaked: %v", m.actions.inflight)
	}
}

func TestInflightLockReleasedOnErrorAndScopedToConnection(t *testing.T) {
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusConflict)
	})
	res, cmd := m.Update(confirmed("pause", "srv"))
	m = res.(Model)
	// A different connection (new client) is a different lock scope.
	other := m
	c := *m.client
	other.client = &c
	other.client.Compute = &gophercloud.ServiceClient{ProviderClient: m.client.Compute.ProviderClient, Endpoint: m.client.Compute.Endpoint}
	if _, cmd2 := other.Update(confirmed("pause", "srv")); cmd2 == nil {
		t.Fatal("lock leaked across connections")
	} else {
		deliver(t, other, cmd2)
	}
	m, _ = deliver(t, m, cmd)
	if m.activeModal != modalError {
		t.Fatal("error result not shown")
	}
	m.activeModal = modalNone
	if _, again := m.Update(confirmed("pause", "srv")); again == nil {
		t.Fatal("lock not released after error")
	}
}
