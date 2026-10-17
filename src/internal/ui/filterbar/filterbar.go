// Package filterbar is the "/" filter input shared by the list views: a
// text input that filters as the user types, is committed with enter and
// cleared with esc, and renders as one line.
package filterbar

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/larkly/lazystack/internal/shared"
)

// Model is a filter bar. The zero value is not usable; call New.
type Model struct {
	input  textinput.Model
	active bool
	width  int
}

// New creates an inactive, empty filter bar.
func New() Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "filter..."
	ti.CharLimit = 64
	ti.SetVirtualCursor(false)
	return Model{input: ti}
}

// Active reports whether the filter input has focus, so every key belongs
// to it rather than to the view or the global shortcuts.
func (m Model) Active() bool { return m.active }

// Value is the current filter text, typed or committed.
func (m Model) Value() string { return m.input.Value() }

// Visible reports whether the bar has anything to show: it is being typed
// in or a filter is committed.
func (m Model) Visible() bool { return m.active || m.input.Value() != "" }

// Start focuses the input to edit the current filter.
func (m *Model) Start() tea.Cmd {
	m.active = true
	return m.input.Focus()
}

// SetValue replaces the filter text (e.g. a saved filter being loaded).
func (m *Model) SetValue(v string) { m.input.SetValue(v) }

// Clear removes a committed filter and reports whether there was one.
func (m *Model) Clear() bool {
	if m.input.Value() == "" {
		return false
	}
	m.input.SetValue("")
	return true
}

// SetWidth sets the width View truncates its line to; 0 leaves it uncut.
func (m *Model) SetWidth(w int) { m.width = w }

// Update handles a key while the bar is active: enter commits the filter,
// esc clears it, and anything else edits it. The view re-applies its filter
// after every call.
func (m Model) Update(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Enter):
		m.active = false
		m.input.Blur()
		return m, nil
	case key.Matches(msg, shared.Keys.Back):
		m.active = false
		m.input.SetValue("")
		m.input.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// Match reports whether any of fields contains the filter text, ignoring
// case. An empty filter matches everything.
func (m Model) Match(fields ...string) bool {
	q := strings.ToLower(m.input.Value())
	if q == "" {
		return true
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

// View renders the bar as one line: the input while editing, the committed
// filter with how to change it otherwise, and "" when there is no filter.
func (m Model) View() string {
	var line string
	switch {
	case m.active:
		line = "  / " + m.input.View()
	case m.input.Value() != "":
		line = shared.StyleHelp.Render("  filter: " + m.input.Value() + " (/ edit, esc clear)")
	default:
		return ""
	}
	if m.width > 0 && lipgloss.Width(line) > m.width {
		line = shared.TruncateCells(line, m.width)
	}
	return line
}

// Inline renders the filter for a panel title: the input while editing and
// the committed filter as "/text"; "" when there is no filter.
func (m Model) Inline() string {
	switch {
	case m.active:
		return "/ " + m.input.View()
	case m.input.Value() != "":
		return lipgloss.NewStyle().Foreground(shared.ColorHighlight).Render("/" + m.input.Value())
	}
	return ""
}
