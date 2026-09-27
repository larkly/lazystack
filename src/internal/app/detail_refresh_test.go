package app

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
)

// detailRefreshFixture answers GET /servers/{id} with a name that encodes
// the server ID and a per-request counter, so the test can tell which
// response was applied.
func detailRefreshFixture(t *testing.T) Model {
	t.Helper()
	var mu sync.Mutex
	n := 0
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		count := n
		mu.Unlock()
		id := strings.TrimPrefix(r.URL.Path, "/servers/")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"server":{"id":%q,"name":"%s-resp-%d","status":"ACTIVE","flavor":{"id":"f"}}}`, id, id, count)
	})
	m.view = viewServerDetail
	return m
}

func openDetail(m Model, id string) Model {
	m.serverDetail = serverdetail.New(m.client.Compute, m.client.Network, m.client.BlockStorage, id, time.Hour)
	m.view = viewServerDetail
	return m
}

func refreshCmd(t *testing.T, m Model, id string) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(delayedDetailRefreshMsg{id: id})
	if cmd == nil {
		t.Fatal("delayed refresh did not start a request")
	}
	return next.(Model), cmd
}

func TestDelayedDetailRefreshForAIsNotAppliedToB(t *testing.T) {
	m := openDetail(detailRefreshFixture(t), "A")
	m, cmd := refreshCmd(t, m, "A")
	m = openDetail(m, "B") // navigate while A's GET is in flight
	result := cmd()
	next, _ := m.Update(result)
	m = next.(Model)
	if m.serverDetail.ServerID() != "B" || m.serverDetail.Server() != nil {
		t.Fatalf("A's refresh reached B: id=%s server=%+v", m.serverDetail.ServerID(), m.serverDetail.Server())
	}
	// Reopening the same server creates a new view: an older view's
	// refresh does not apply to it either.
	m = openDetail(m, "A")
	m, cmd = refreshCmd(t, m, "A")
	stale := cmd()
	m = openDetail(m, "A")
	next, _ = m.Update(stale)
	if next.(Model).serverDetail.Server() != nil {
		t.Fatal("refresh for an earlier detail view of the same server was applied")
	}
}

func TestOverlappingDetailRefreshesKeepNewest(t *testing.T) {
	m := openDetail(detailRefreshFixture(t), "S")
	m, first := refreshCmd(t, m, "S")
	m, second := refreshCmd(t, m, "S")
	older, newer := first(), second()
	next, _ := m.Update(newer)
	m = next.(Model)
	next, _ = m.Update(older)
	m = next.(Model)
	if got := m.serverDetail.ServerName(); got != "S-resp-2" {
		t.Fatalf("detail shows %q, want the newest refresh S-resp-2", got)
	}
}
