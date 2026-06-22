package auditlog

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/shared"
)

// FilterType is the active filter mode.
type FilterType int

const (
	FilterNone FilterType = iota
	FilterAction
	FilterResource
	FilterDate
)

// Model is the audit log viewer.
type Model struct {
	entries       []audit.Entry
	filtered      []audit.Entry
	cursor        int
	scroll        int // top visible index
	loading       bool
	spinner       spinner.Model
	err           string
	width         int
	height        int
	filterMode    FilterType
	filterInput   string
	filterCursor  int
}

// New creates an audit log viewer.
func New() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		spinner: s,
	}
}

// SetEntries loads entries into the viewer.
func (m *Model) SetEntries(entries []audit.Entry) {
	m.entries = entries
	m.applyFilter()
	m.loading = false
}

// SetError sets an error message.
func (m *Model) SetError(err string) {
	m.err = err
	m.loading = false
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Init starts loading.
func (m Model) Init() tea.Cmd {
	m.loading = true
	return m.spinner.Tick
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.filterMode != FilterNone {
			return m.updateFilterInput(msg)
		}
		switch {
		case key.Matches(msg, shared.Keys.Back), key.Matches(msg, shared.Keys.Back):
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "serverlist"}
			}
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.adjustScroll()
			}
			return m, nil
		case msg.String() == "a":
			m.filterMode = FilterAction
			m.filterInput = ""
			m.filterCursor = 0
			return m, nil
		case msg.String() == "r":
			m.filterMode = FilterResource
			m.filterInput = ""
			m.filterCursor = 0
			return m, nil
		case msg.String() == "d":
			m.filterMode = FilterDate
			m.filterInput = ""
			m.filterCursor = 0
			return m, nil
		case msg.String() == "c":
			m.filterMode = FilterNone
			m.filterInput = ""
			m.applyFilter()
			return m, nil
		}
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

func (m Model) updateFilterInput(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back), key.Matches(msg, shared.Keys.Back):
		m.filterMode = FilterNone
		m.filterInput = ""
		m.applyFilter()
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		m.applyFilter()
		m.filterMode = FilterNone
		return m, nil
	case msg.String() == "backspace" || msg.String() == "delete":
		if len(m.filterInput) > 0 {
			m.filterInput = m.filterInput[:len(m.filterInput)-1]
			m.applyFilter()
		}
		return m, nil
	case msg.String() == "ctrl+u":
		m.filterInput = ""
		m.applyFilter()
		return m, nil
	default:
		r := msg.String()
		if len(r) == 1 && r[0] >= 32 && r[0] < 127 {
			m.filterInput += r
			m.applyFilter()
		}
		return m, nil
	}
}

func (m *Model) applyFilter() {
	if m.filterInput == "" && m.filterMode == FilterNone {
		m.filtered = append([]audit.Entry(nil), m.entries...)
		return
	}
	query := strings.ToLower(m.filterInput)
	var out []audit.Entry
	for _, e := range m.entries {
		match := false
		switch m.filterMode {
		case FilterAction:
			match = strings.Contains(strings.ToLower(string(e.Action)), query)
		case FilterResource:
			match = strings.Contains(strings.ToLower(e.ResourceType), query)
		case FilterDate:
			match = strings.Contains(e.Timestamp.Format("2006-01-02"), query)
		default:
			match = strings.Contains(strings.ToLower(string(e.Action)), query) ||
				strings.Contains(strings.ToLower(e.ResourceType), query) ||
				strings.Contains(strings.ToLower(e.ResourceName), query)
		}
		if match {
			out = append(out, e)
		}
	}
	m.filtered = out
	m.cursor = 0
	m.scroll = 0
}

func (m *Model) adjustScroll() {
	visible := m.visibleRows()
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	} else if m.cursor >= m.scroll+visible {
		m.scroll = m.cursor - visible + 1
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

func (m Model) visibleRows() int {
	headerRows := 5 // title + help + separator + column headers + separator
	if m.err != "" {
		headerRows++
	}
	if m.filterMode != FilterNone {
		headerRows++
	}
	avail := m.height - headerRows - 2
	if avail < 1 {
		return 1
	}
	return avail
}

// View renders the audit log table.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Audit Log")
	if m.loading {
		title += " " + m.spinner.View() + shared.StyleHelp.Render(" loading...")
	}
	b.WriteString(title + "\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  ⚠ "+m.err) + "\n")
	}

	filterLabel := ""
	switch m.filterMode {
	case FilterAction:
		filterLabel = "[a]ction filter: " + m.filterInput + "_"
	case FilterResource:
		filterLabel = "[r]esource filter: " + m.filterInput + "_"
	case FilterDate:
		filterLabel = "[d]ate filter: " + m.filterInput + "_"
	}
	if filterLabel != "" {
		b.WriteString(shared.StyleHelp.Render("  "+filterLabel) + "\n")
	}

	b.WriteString(shared.StyleHelp.Render("  a=action r=resource d=date c=clear ↑↓ navigate esc=back") + "\n")
	b.WriteString("  " + strings.Repeat("─", m.width-4) + "\n")

	// Column headers
	cols := []struct{ w int; title string }{
		{20, "Timestamp"},
		{12, "Action"},
		{14, "Resource"},
		{24, "Name"},
		{10, "Result"},
	}
	for i, c := range cols {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(lipgloss.NewStyle().Width(c.w).Bold(true).Render(c.title))
	}
	b.WriteString("\n")
	b.WriteString("  " + strings.Repeat("─", m.width-4) + "\n")

	if len(m.filtered) == 0 {
		b.WriteString("  " + shared.StyleHelp.Render("No entries") + "\n")
		return b.String()
	}

	visible := m.visibleRows()
	end := m.scroll + visible
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := m.scroll; i < end; i++ {
		e := m.filtered[i]
		cursor := "  "
		if i == m.cursor {
			cursor = "▸ "
		}
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.cursor {
			style = style.Foreground(shared.ColorHighlight).Bold(true)
		}

		resultStyle := lipgloss.NewStyle().Foreground(shared.ColorSuccess)
		if e.Result != "success" {
			resultStyle = lipgloss.NewStyle().Foreground(shared.ColorError)
		}

		row := fmt.Sprintf(
			"%s%s %s %s %s %s\n",
			cursor,
			lipgloss.NewStyle().Width(cols[0].w).Render(e.Timestamp.Format("2006-01-02 15:04")),
			lipgloss.NewStyle().Width(cols[1].w).Render(string(e.Action)),
			lipgloss.NewStyle().Width(cols[2].w).Render(e.ResourceType),
			lipgloss.NewStyle().Width(cols[3].w).Render(e.ResourceName),
			resultStyle.Width(cols[4].w).Render(e.Result),
		)
		b.WriteString(style.Render(row))
	}

	return b.String()
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "↑↓ navigate • a action • r resource • d date • c clear • esc back"
}
