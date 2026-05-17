package columnpicker

import (
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/config"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ColumnsChosenMsg is emitted when the user confirms column selection.
type ColumnsChosenMsg struct {
	Columns []config.ColumnConfig
}

// ColumnsCancelledMsg is emitted when the user dismisses the picker.
type ColumnsCancelledMsg struct{}

// PickerColumn is a displayable column entry in the picker.
type PickerColumn struct {
	Title  string
	Key    string
	Hidden bool
}

// Model is the column picker overlay.
type Model struct {
	columns []PickerColumn
	cursor  int
	Active  bool
	width   int
	height  int
}

// New creates a column picker from a list of serverlist Column values.
// The caller should pass the current columns slice; Hidden will be set
// according to whether the column is currently visible.
func New(current []PickerColumn) Model {
	return Model{
		columns: current,
		Active:  true,
	}
}

// Update handles input for the column picker.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Back):
			m.Active = false
			return m, func() tea.Msg { return ColumnsCancelledMsg{} }
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.columns)-1 {
				m.cursor++
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Left):
			if m.cursor > 0 {
				m.columns[m.cursor], m.columns[m.cursor-1] = m.columns[m.cursor-1], m.columns[m.cursor]
				m.cursor--
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Right):
			if m.cursor < len(m.columns)-1 {
				m.columns[m.cursor], m.columns[m.cursor+1] = m.columns[m.cursor+1], m.columns[m.cursor]
				m.cursor++
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Select):
			if m.cursor >= 0 && m.cursor < len(m.columns) {
				m.columns[m.cursor].Hidden = !m.columns[m.cursor].Hidden
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			m.Active = false
			configs := make([]config.ColumnConfig, len(m.columns))
			for i, c := range m.columns {
				configs[i] = config.ColumnConfig{Key: c.Key, Hidden: c.Hidden}
			}
			return m, func() tea.Msg { return ColumnsChosenMsg{Columns: configs} }
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

// View renders the column picker modal.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Columns")
	var lines []string
	for i, c := range m.columns {
		cursor := "  "
		check := "[ ]"
		if i == m.cursor {
			cursor = "▸ "
		}
		if !c.Hidden {
			check = "[x]"
		}
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.cursor {
			style = style.Foreground(shared.ColorHighlight).Bold(true)
		}
		lines = append(lines, fmt.Sprintf("%s%s %s", cursor, check, style.Render(c.Title)))
	}
	body := strings.Join(lines, "\n")
	hint := shared.StyleHelp.Render("↑↓ select • ←→ reorder • space toggle • enter apply • esc cancel")
	content := title + "\n\n" + body + "\n\n" + hint
	box := shared.StyleModal.Width(40).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// FromServerColumns converts a slice of full serverlist Column structs
// into PickerColumn entries. The caller should ensure the Hidden flag
// is set correctly on the source columns before calling.
func FromServerColumns(serverCols []struct {
	Title    string
	MinWidth int
	Flex     int
	Priority int
	Key      string
	Width    int
	Hidden   bool
}) []PickerColumn {
	out := make([]PickerColumn, len(serverCols))
	for i, c := range serverCols {
		out[i] = PickerColumn{Title: c.Title, Key: c.Key, Hidden: c.Hidden}
	}
	return out
}
