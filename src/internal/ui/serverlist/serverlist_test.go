package serverlist

import (
	"strings"
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
