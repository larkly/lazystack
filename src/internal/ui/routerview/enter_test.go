package routerview

import (
	"strings"
	"testing"
)

// Enter on a router opens its detail: focus moves to the interfaces pane.
func TestEnterOnRouterFocusesInterfaces(t *testing.T) {
	m := loadedRouters(t)
	m, _ = m.Update(enterKey)
	if !m.InInterfaces() {
		t.Fatalf("focus = %v after enter, want the interfaces pane", m.focus)
	}
}

func TestEnterWithNoRouterDoesNothing(t *testing.T) {
	m := loadedRouters(t)
	m = typeFilter(m, "zzz")
	m, _ = m.Update(enterKey)
	if m.focus != FocusSelector {
		t.Fatalf("focus = %v after enter on an empty list, want the router list", m.focus)
	}
}

// Esc from the detail returns focus to the router list, after it has
// cleared a filter; on the list itself esc is left to the root model.
func TestEscapeReturnsFromDetailAfterClearingFilter(t *testing.T) {
	m := loadedRouters(t)
	m = typeFilter(m, "edge")
	m, _ = m.Update(enterKey)
	if !m.InInterfaces() {
		t.Fatal("setup: enter did not focus interfaces")
	}
	if !m.Escape() || m.FilterValue() != "" || !m.InInterfaces() {
		t.Fatalf("first escape: filter=%q focus=%v, want filter cleared and focus kept", m.FilterValue(), m.focus)
	}
	if !m.Escape() || m.focus != FocusSelector {
		t.Fatalf("second escape: focus=%v, want the router list", m.focus)
	}
	if m.Escape() {
		t.Fatal("escape on the list with nothing to clear reported handled")
	}
}

func TestDetailHintsSayEscReturnsToList(t *testing.T) {
	m := loadedRouters(t)
	m, _ = m.Update(enterKey)
	if h := m.Hints(); !strings.Contains(h, "esc list") {
		t.Fatalf("interfaces hints %q do not say esc returns to the list", h)
	}
	if h := m.Hints(); strings.Contains(h, "esc back") {
		t.Fatalf("interfaces hints %q say esc back", h)
	}
}
