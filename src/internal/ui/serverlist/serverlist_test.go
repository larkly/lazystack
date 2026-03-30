package serverlist

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/compute"
)

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
