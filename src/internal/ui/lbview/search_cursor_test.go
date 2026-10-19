package lbview

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/loadbalancer"
)

func typeKeys(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func selectedName(m Model) string {
	if lb := m.SelectedLB(); lb != nil {
		return lb.Name
	}
	return ""
}

// Typing a search and pressing Enter keeps the cursor on the load balancer it
// was on while it still matches, and does not reload its detail.
func TestSearchKeepsCursorOnSelectedLB(t *testing.T) {
	m := New(nil, time.Second)
	m.SetSize(120, 40)
	m, _ = m.Update(lbsLoadedMsg{lbs: []loadbalancer.LoadBalancer{
		{ID: "a", Name: "web-1"}, {ID: "b", Name: "db-1"}, {ID: "c", Name: "web-2"}, {ID: "d", Name: "db-2"},
	}})
	for i := 0; i < 10 && selectedName(m) != "web-2"; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if selectedName(m) != "web-2" {
		t.Fatalf("setup: cursor on %q", selectedName(m))
	}
	m.detailLoading = false

	m = typeKeys(m, "/")
	for _, r := range "web" {
		m = typeKeys(m, string(r))
		if got := selectedName(m); got != "web-2" {
			t.Fatalf("after typing %q the cursor is on %q, want web-2", m.searchFilter, got)
		}
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := selectedName(m); got != "web-2" {
		t.Fatalf("Enter moved the cursor to %q, want web-2", got)
	}
	if m.detailLoading {
		t.Fatal("keeping the same load balancer reloaded its detail")
	}

	m = typeKeys(m, "/")
	m = typeKeys(m, "x")
	if m.cursor != 0 {
		t.Fatalf("cursor=%d after the selected load balancer was filtered out, want 0", m.cursor)
	}
}
