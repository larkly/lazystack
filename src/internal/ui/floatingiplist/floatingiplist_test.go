package floatingiplist

import (
	"errors"
	"testing"
	"time"

	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
)

func TestDescendingSortKeepsTiesStable(t *testing.T) {
	m := New(nil, time.Second)
	m.fips = []network.FloatingIP{
		{ID: "1", FloatingIP: "2001:db8::1", Status: "DOWN"},
		{ID: "2", FloatingIP: "2001:db8::2", Status: "ACTIVE"},
		{ID: "3", FloatingIP: "2001:db8::3", Status: "DOWN"},
		{ID: "4", FloatingIP: "2001:db8::4", Status: "DOWN"},
	}
	m.sortCol = 1 // status
	m.sortAsc = false
	m.sortFIPs()

	want := []string{"1", "3", "4", "2"}
	for i, f := range m.fips {
		if f.ID != want[i] {
			var got []string
			for _, g := range m.fips {
				got = append(got, g.ID)
			}
			t.Fatalf("descending by status = %v, want %v (ties must keep input order)", got, want)
		}
	}
}

// Background ticks must not stack list fetches, and a slow older response
// must never overwrite the result of a newer fetch.
func TestTickRefreshesDoNotOverlapOrRegress(t *testing.T) {
	m := New(nil, time.Second)
	m.loading = false

	m, first := m.Update(shared.TickMsg{})
	if first == nil {
		t.Fatal("tick did not fetch")
	}
	staleSeq := m.refresh.Seq()
	if _, again := m.Update(shared.TickMsg{}); again != nil {
		t.Fatal("tick started a second fetch while one was in flight")
	}

	// A manual refresh supersedes the slow tick fetch.
	m.ForceRefresh()
	newSeq := m.refresh.Seq()
	m, _ = m.Update(fipsLoadedMsg{seq: newSeq, fips: []network.FloatingIP{{ID: "new"}}})
	m, _ = m.Update(fipsLoadedMsg{seq: staleSeq, fips: []network.FloatingIP{{ID: "old"}}})
	m, _ = m.Update(fipsErrMsg{seq: staleSeq, err: errors.New("stale failure")})
	if len(m.fips) != 1 || m.fips[0].ID != "new" || m.err != "" {
		t.Fatalf("stale response applied: %v err=%q", m.fips, m.err)
	}
	if _, next := m.Update(shared.TickMsg{}); next == nil {
		t.Fatal("tick blocked after the newest fetch completed")
	}
}
