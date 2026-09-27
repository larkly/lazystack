package serverlist

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
)

func TestDescendingSortKeepsTiesStable(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.filtered = []compute.Server{
		{ID: "s-1", Name: "a", Status: "SHUTOFF"},
		{ID: "s-2", Name: "b", Status: "ACTIVE"},
		{ID: "s-3", Name: "c", Status: "SHUTOFF"},
		{ID: "s-4", Name: "d", Status: "SHUTOFF"},
	}
	m.sortCol = -1
	for i := 0; m.visibleColKey(i) != ""; i++ {
		if m.visibleColKey(i) == "status" {
			m.sortCol = i
		}
	}
	if m.sortCol < 0 {
		t.Fatal("status column not visible by default")
	}
	m.sortAsc = false
	m.sortServers()

	want := []string{"s-1", "s-3", "s-4", "s-2"}
	for i, s := range m.filtered {
		if s.ID != want[i] {
			var got []string
			for _, g := range m.filtered {
				got = append(got, g.ID)
			}
			t.Fatalf("descending by status = %v, want %v (ties must keep input order)", got, want)
		}
	}
}

func TestEscClearsFilterAndExitsFiltering(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.servers = []compute.Server{
		{ID: "s-1", Name: "alpha"},
		{ID: "s-2", Name: "beta"},
	}
	m.applyFilter()

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: '/'}))
	m = updated
	if !m.filtering {
		t.Fatalf("expected filtering mode to be enabled")
	}

	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}))
	m = updated
	if got := m.filter.Value(); got != "a" {
		t.Fatalf("filter value = %q, want %q", got, "a")
	}

	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m = updated

	if m.filtering {
		t.Fatalf("expected filtering mode to be disabled after esc")
	}
	if got := m.filter.Value(); got != "" {
		t.Fatalf("filter value = %q, want empty after esc", got)
	}
	if len(m.filtered) != len(m.servers) {
		t.Fatalf("filtered len = %d, want %d after clear", len(m.filtered), len(m.servers))
	}
}

func TestEscClearsSelectionAsAdvertised(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.servers = []compute.Server{
		{ID: "s-1", Name: "alpha"},
		{ID: "s-2", Name: "beta"},
	}
	m.applyFilter()
	for i := 0; i < 2; i++ {
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
	}
	if m.SelectionCount() != 2 || !strings.Contains(m.Hints(), "esc clear") {
		t.Fatalf("setup: selection=%d hints=%q", m.SelectionCount(), m.Hints())
	}
	m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if m.SelectionCount() != 0 || cmd != nil {
		t.Fatalf("esc: selection=%d cmd=%v, want cleared with no command", m.SelectionCount(), cmd != nil)
	}
}

func TestRenderServerRow_IPv6SuffixTruncation(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.columns = []Column{
		{Title: "IPv6", MinWidth: 15, Flex: 0, Priority: 0, Key: "ipv6"},
	}
	m.columns = ComputeWidths(m.columns, 40, nil)

	longIPv6 := "2001:db8:85a3:0000:0000:8a2e:0370:7334"
	s := compute.Server{ID: "s-1", Name: "n", IPv6: []string{longIPv6}}

	row := m.renderServerRow(s, false)

	if !strings.Contains(row, "…") {
		t.Errorf("expected ellipsis in row, got %q", row)
	}
	suffix := longIPv6[len(longIPv6)-5:]
	if !strings.Contains(row, suffix) {
		t.Errorf("expected suffix %q preserved in row, got %q", suffix, row)
	}
	prefix := longIPv6[:5]
	if strings.Contains(row, prefix) {
		t.Errorf("expected prefix %q to be truncated out, but found it in %q", prefix, row)
	}
}

func TestRenderServerRow_OtherColumnsPrefixTruncation(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.columns = []Column{
		{Title: "Name", MinWidth: 10, Flex: 0, Priority: 0, Key: "name"},
	}
	m.columns = ComputeWidths(m.columns, 30, nil)

	longName := "very-long-server-name-here"
	s := compute.Server{ID: "s-1", Name: longName}

	row := m.renderServerRow(s, false)

	if !strings.Contains(row, "…") {
		t.Errorf("expected ellipsis in row, got %q", row)
	}
	if !strings.Contains(row, longName[:5]) {
		t.Errorf("expected prefix preserved for non-ipv6 column, got %q", row)
	}
}

// Background ticks must not stack list fetches, and a slow older response
// must never overwrite the result of a newer fetch.
func TestTickRefreshesDoNotOverlapOrRegress(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
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
	m, _ = m.Update(serversLoadedMsg{seq: newSeq, servers: []compute.Server{{ID: "new", Name: "new"}}})
	m, _ = m.Update(serversLoadedMsg{seq: staleSeq, servers: []compute.Server{{ID: "old", Name: "old"}}})
	m, _ = m.Update(serversErrMsg{seq: staleSeq, err: errors.New("stale failure")})
	if len(m.servers) != 1 || m.servers[0].ID != "new" || m.err != "" {
		t.Fatalf("stale response applied: servers=%v err=%q", m.servers, m.err)
	}
	if _, next := m.Update(shared.TickMsg{}); next == nil {
		t.Fatal("tick blocked after the newest fetch completed")
	}
}
