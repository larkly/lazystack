package keypairlist

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/larkly/lazystack/internal/compute"
)

func runeKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}) }

var (
	enterKey = tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	escKey   = tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	downKey  = tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
)

func filterPairs() []compute.KeyPair {
	return []compute.KeyPair{
		{Name: "alice-laptop", Type: "ssh"},
		{Name: "bob", Type: "ssh"},
		{Name: "alice-ci", Type: "x509"},
	}
}

func typeFilter(m Model, q string) Model {
	m, _ = m.Update(runeKey('/'))
	for _, r := range q {
		m, _ = m.Update(runeKey(r))
	}
	m, _ = m.Update(enterKey)
	return m
}

func TestFilterNarrowsListAndPersistsAcrossRefresh(t *testing.T) {
	m := New(nil, time.Second)
	m.SetSize(120, 30)
	m, _ = m.Update(keypairsLoadedMsg{keypairs: filterPairs()})
	m = typeFilter(m, "ALICE")
	if m.IsFiltering() || m.FilterValue() != "ALICE" || len(m.pairs) != 2 {
		t.Fatalf("filter: active=%v value=%q listed=%d, want committed with 2 rows", m.IsFiltering(), m.FilterValue(), len(m.pairs))
	}
	m, _ = m.Update(downKey)
	want := m.SelectedKeyPair().Name
	m, _ = m.Update(keypairsLoadedMsg{keypairs: append(filterPairs(), compute.KeyPair{Name: "alice-new"})})
	if len(m.pairs) != 3 || m.SelectedKeyPair().Name != want {
		t.Fatalf("after refresh: listed=%d cursor=%s, want 3 rows and cursor on %s", len(m.pairs), m.SelectedKeyPair().Name, want)
	}
	if !strings.Contains(ansi.Strip(m.View()), "filter: ALICE") {
		t.Fatal("committed filter not shown")
	}
}

func TestFilterMatchesType(t *testing.T) {
	m := New(nil, time.Second)
	m.SetSize(120, 30)
	m, _ = m.Update(keypairsLoadedMsg{keypairs: filterPairs()})
	m = typeFilter(m, "x509")
	if len(m.pairs) != 1 || m.pairs[0].Name != "alice-ci" {
		t.Fatalf("listed %+v, want only alice-ci", m.pairs)
	}
}

func TestEscClearsCommittedFilter(t *testing.T) {
	m := New(nil, time.Second)
	m.SetSize(120, 30)
	m, _ = m.Update(keypairsLoadedMsg{keypairs: filterPairs()})
	m = typeFilter(m, "bob")
	m, _ = m.Update(escKey)
	if m.FilterValue() != "" || len(m.pairs) != 3 {
		t.Fatalf("esc: filter=%q listed=%d, want cleared and 3", m.FilterValue(), len(m.pairs))
	}
}

func TestFilterWithNoMatchesSaysSo(t *testing.T) {
	m := New(nil, time.Second)
	m.SetSize(120, 30)
	m, _ = m.Update(keypairsLoadedMsg{keypairs: filterPairs()})
	m = typeFilter(m, "zzz")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No key pairs match") {
		t.Fatalf("view does not say nothing matches:\n%s", v)
	}
}

// A key pair just created must get the cursor even when the committed
// filter hides it: the filter gives way.
func TestHighlightClearsFilterThatHidesTarget(t *testing.T) {
	m := New(nil, time.Second)
	m.SetSize(120, 30)
	m, _ = m.Update(keypairsLoadedMsg{keypairs: filterPairs()})
	m = typeFilter(m, "alice")
	m.SetHighlight("bob")
	if m.FilterValue() != "" || m.SelectedKeyPair() == nil || m.SelectedKeyPair().Name != "bob" {
		t.Fatalf("filter=%q cursor=%+v, want filter cleared and cursor on bob", m.FilterValue(), m.SelectedKeyPair())
	}
}
