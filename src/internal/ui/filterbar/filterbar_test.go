package filterbar

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	return m
}

var (
	enter = tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	esc   = tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
)

func TestNewIsInactiveAndEmpty(t *testing.T) {
	m := New()
	if m.Active() || m.Value() != "" || m.Visible() || m.View() != "" || m.Inline() != "" {
		t.Fatalf("new bar not blank: active=%v value=%q view=%q", m.Active(), m.Value(), m.View())
	}
}

func TestTypeAndCommitWithEnter(t *testing.T) {
	m := New()
	m.Start()
	if !m.Active() {
		t.Fatal("Start did not activate the bar")
	}
	m = typeText(m, "web")
	if m.Value() != "web" {
		t.Fatalf("value = %q, want web", m.Value())
	}
	if got := ansi.Strip(m.View()); !strings.HasPrefix(got, "  / ") {
		t.Fatalf("active view = %q, want the input line", got)
	}
	m, _ = m.Update(enter)
	if m.Active() || m.Value() != "web" || !m.Visible() {
		t.Fatalf("enter: active=%v value=%q, want committed web", m.Active(), m.Value())
	}
	if got := ansi.Strip(m.View()); got != "  filter: web (/ edit, esc clear)" {
		t.Fatalf("committed view = %q", got)
	}
	if got := ansi.Strip(m.Inline()); got != "/web" {
		t.Fatalf("committed inline = %q", got)
	}
}

func TestEscWhileTypingClearsAndLeaves(t *testing.T) {
	m := New()
	m.Start()
	m = typeText(m, "x")
	m, _ = m.Update(esc)
	if m.Active() || m.Value() != "" {
		t.Fatalf("esc: active=%v value=%q, want inactive and empty", m.Active(), m.Value())
	}
}

func TestClearReportsWhetherAnythingWasCleared(t *testing.T) {
	m := New()
	if m.Clear() {
		t.Fatal("Clear on empty bar reported a change")
	}
	m.SetValue("db")
	if !m.Clear() || m.Value() != "" {
		t.Fatalf("Clear did not remove the committed filter: %q", m.Value())
	}
}

func TestMatchIsCaseInsensitiveSubstringOverAnyField(t *testing.T) {
	m := New()
	if !m.Match("anything") || !m.Match() {
		t.Fatal("empty filter must match everything")
	}
	m.SetValue("WeB")
	if !m.Match("db", "my-web-01") {
		t.Fatal("expected a match in the second field")
	}
	if m.Match("db", "cache") {
		t.Fatal("unexpected match")
	}
}

func TestViewTruncatesToWidth(t *testing.T) {
	m := New()
	m.SetValue(strings.Repeat("a", 60))
	m.SetWidth(30)
	if w := ansi.StringWidth(m.View()); w > 30 {
		t.Fatalf("view width %d exceeds 30", w)
	}
}
