package projectpicker

import (
	"fmt"

	"github.com/bosse/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Model is the project picker overlay.
type Model struct {
	projects  []shared.ProjectInfo
	currentID string
	cursor    int
	Active    bool
	width     int
	height    int
}

// New creates a project picker with the given projects.
func New(projects []shared.ProjectInfo, currentID string) Model {
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
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}
		case key.Matches(msg, shared.Keys.Enter):
			if len(m.projects) > 0 {
				selected := m.projects[m.cursor]
				if selected.ID == m.currentID {
					// Already on this project, just close
					m.Active = false
					return m, nil
				}
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

// View renders the project picker.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Switch Project")
	items := ""
	for i, p := range m.projects {
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
		items += fmt.Sprintf("%s%s%s\n", cursor, marker, style.Render(p.Name))
	}

	hint := shared.StyleHelp.Render("↑/↓ navigate • enter select • esc cancel")

	content := title + "\n\n" + items + "\n" + hint
	box := shared.StyleModal.Width(40).Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}
