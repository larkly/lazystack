package projectpicker

import (
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Model is the project picker overlay.
type Model struct {
	projects  []shared.ProjectInfo
	currentID string
	cursor    int
	scroll    int
	Active    bool
	width     int
	height    int
}

// New creates a project picker with the given projects.
func New(projects []shared.ProjectInfo, currentID string) Model {
	shared.Debugf("[projectpicker] Init() projects=%d currentID=%s", len(projects), currentID)
	// Position cursor on the current project
	cursor := 0
	for i, p := range projects {
		if p.ID == currentID {
			cursor = i
			break
		}
	}
	return Model{
		projects:  projects,
		currentID: currentID,
		cursor:    cursor,
		scroll:    0,
		Active:    true,
	}
}

// Update handles input for the project picker.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
			m.adjustScroll()
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}
			m.adjustScroll()
		case key.Matches(msg, shared.Keys.Enter):
			if len(m.projects) > 0 {
				selected := m.projects[m.cursor]
				if selected.ID == m.currentID {
					// Already on this project, just close
					shared.Debugf("[projectpicker] selected current project, closing")
					m.Active = false
					return m, nil
				}
				shared.Debugf("[projectpicker] selected project=%q id=%s", selected.Name, selected.ID)
				m.Active = false
				return m, func() tea.Msg {
					return shared.ProjectSelectedMsg{
						ProjectID:   selected.ID,
						ProjectName: selected.Name,
					}
				}
			}
		case key.Matches(msg, shared.Keys.Back):
			m.Active = false
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m *Model) adjustScroll() {
	visible := m.visibleProjects()
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if m.cursor >= m.scroll+visible {
		m.scroll = m.cursor - visible + 1
	}
}

func (m Model) visibleProjects() int {
	// Title + warning + padding + hint ≈ 6 lines; rest are items.
	v := m.height - 8
	if v < 1 {
		v = 1
	}
	if v > len(m.projects) {
		v = len(m.projects)
	}
	return v
}

// View renders the project picker.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Switch Project")
	warning := lipgloss.NewStyle().Foreground(shared.ColorWarning).Bold(true).Render("ALPHA — UNTESTED")

	visible := m.visibleProjects()
	start := m.scroll
	end := start + visible
	if end > len(m.projects) {
		end = len(m.projects)
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		p := m.projects[i]
		cursor := "  "
		marker := "  "
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.cursor {
			cursor = "▸ "
			style = lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
		}
		if p.ID == m.currentID {
			marker = "* "
		}
		b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, marker, style.Render(p.Name)))
	}

	hint := shared.StyleHelp.Render("↑/↓ navigate • enter select • esc cancel")
	content := title + "\n" + warning + "\n\n" + b.String() + "\n" + hint

	if len(m.projects) > visible {
		pageHint := shared.StyleHelp.Render(fmt.Sprintf("(%d–%d of %d)", start+1, end, len(m.projects)))
		content = title + "\n" + warning + "\n\n" + b.String() + pageHint + "\n" + hint
	}

	box := shared.StyleModal.Width(40).Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}
