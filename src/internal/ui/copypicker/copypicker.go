package copypicker

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/larkly/lazystack/internal/shared"
)

// Entry is a single copyable field.
type Entry struct {
	Label string
	Value string
}

// Builder is a tiny helper for composing []Entry while skipping empty values.
// It lets view code write:
//
//	b := copypicker.Builder{}
//	b.Add("ID", s.ID).Add("Name", s.Name).AddEach("Floating IP", s.FloatingIP)
//	return b.Entries()
type Builder struct {
	entries []Entry
}

// Add appends a single entry, skipping empty values.
func (b *Builder) Add(label, value string) *Builder {
	if value == "" {
		return b
	}
	b.entries = append(b.entries, Entry{Label: label, Value: value})
	return b
}

// AddEach appends one entry per non-empty value. When multiple values
// are present the label is suffixed with the value in parentheses so
// each row is distinguishable.
func (b *Builder) AddEach(label string, values []string) *Builder {
	nonEmpty := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			nonEmpty = append(nonEmpty, v)
		}
	}
	if len(nonEmpty) == 0 {
		return b
	}
	if len(nonEmpty) == 1 {
		b.entries = append(b.entries, Entry{Label: label, Value: nonEmpty[0]})
		return b
	}
	for _, v := range nonEmpty {
		b.entries = append(b.entries, Entry{Label: fmt.Sprintf("%s (%s)", label, v), Value: v})
	}
	return b
}

// Entries returns the accumulated entries.
func (b *Builder) Entries() []Entry { return b.entries }

// ChosenMsg is emitted when the user selects an entry to copy.
type ChosenMsg struct {
	Label string
	Value string
}

// CancelledMsg is emitted when the user dismisses the picker.
type CancelledMsg struct{}

// Model is a small modal that lists copyable fields and returns the
// selected value via ChosenMsg. It does not touch the clipboard itself —
// the app layer does that so all copy feedback lives in one place.
type Model struct {
	Active  bool
	title   string
	entries []Entry
	cursor  int
	width   int
	height  int
}

// New builds an active picker. Pass a descriptive title (e.g. "Copy — server web01")
// and the entries to show; empty entries are accepted but usually callers
// should skip the picker entirely if there is nothing to copy.
func New(title string, entries []Entry) Model {
	return Model{
		Active:  true,
		title:   title,
		entries: entries,
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Back):
			m.Active = false
			return m, func() tea.Msg { return CancelledMsg{} }
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			return m.choose(m.cursor)
		}
		// Number-row quick pick: 1-9 selects that row directly.
		s := msg.String()
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			idx := int(s[0] - '1')
			if idx < len(m.entries) {
				return m.choose(idx)
			}
		}
	}
	return m, nil
}

func (m Model) choose(idx int) (Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.entries) {
		return m, nil
	}
	e := m.entries[idx]
	m.Active = false
	return m, func() tea.Msg { return ChosenMsg{Label: e.Label, Value: e.Value} }
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m Model) View() string {
	title := shared.StyleModalTitle.Render(m.title)

	labelW, valueW := m.columnWidths()

	var lines []string
	for i, e := range m.entries {
		cursor := "  "
		rowStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.cursor {
			cursor = "▸ "
			rowStyle = rowStyle.Foreground(shared.ColorHighlight).Bold(true)
		}
		num := fmt.Sprintf("%d.", i+1)
		if i >= 9 {
			num = "  "
		}
		label := padRight(e.Label, labelW)
		value := truncate(e.Value, valueW)
		lines = append(lines, cursor+rowStyle.Render(num+" "+label+"  "+value))
	}
	body := strings.Join(lines, "\n")
	body += "\n\n" + shared.StyleHelp.Render("↑↓ navigate • 1-9 quick pick • enter copy • esc cancel")

	content := title + "\n\n" + body
	modalWidth := labelW + valueW + 16
	if modalWidth < 40 {
		modalWidth = 40
	}
	if m.width > 0 && modalWidth > m.width-6 {
		modalWidth = m.width - 6
	}
	box := shared.StyleModal.Width(modalWidth).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) columnWidths() (labelW, valueW int) {
	for _, e := range m.entries {
		if l := len(e.Label); l > labelW {
			labelW = l
		}
		if l := len(e.Value); l > valueW {
			valueW = l
		}
	}
	if valueW > 60 {
		valueW = 60
	}
	return labelW, valueW
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return s[:n-1] + "…"
}
