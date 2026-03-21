package cloudpicker

import (
	"fmt"

	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Model is the cloud picker overlay.
type Model struct {
	clouds   []string
	cursor   int
	width    int
	height   int
	err      error
}

// New creates a cloud picker with the given cloud names.
func New(clouds []string, err error) Model {
	return Model{
		clouds: clouds,
		err:    err,
	}
}

// Init returns no initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles input for the cloud picker.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.clouds)-1 {
				m.cursor++
			}
		case key.Matches(msg, shared.Keys.Enter):
			if len(m.clouds) > 0 {
				return m, func() tea.Msg {
					return shared.CloudSelectedMsg{CloudName: m.clouds[m.cursor]}
				}
			}
		case key.Matches(msg, shared.Keys.Quit):
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

// View renders the cloud picker.
func (m Model) View() string {
	if m.err != nil {
		content := shared.StyleModalTitle.Render("No clouds.yaml found") + "\n\n" +
			lipgloss.NewStyle().Foreground(shared.ColorError).Render(m.err.Error()) + "\n\n" +
			shared.StyleHelp.Render("Create ~/.config/openstack/clouds.yaml or set OS_CLIENT_CONFIG_FILE") + "\n" +
			shared.StyleHelp.Render("Press q to quit")

		box := shared.StyleErrorModal.Width(60).Render(content)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}

	title := shared.StyleModalTitle.Render("Select Cloud")
	items := ""
	for i, name := range m.clouds {
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.cursor {
			cursor = "▸ "
			style = lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
		}
		items += fmt.Sprintf("%s%s\n", cursor, style.Render(name))
	}

	hint := shared.StyleHelp.Render("↑/↓ navigate • enter select • q quit")

	content := title + "\n\n" + items + "\n" + hint
	box := shared.StyleModal.Width(40).Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}
