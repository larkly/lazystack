package help

import (
	"strings"

	"github.com/bosse/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ToggleHelpMsg toggles the help overlay.
type ToggleHelpMsg struct{}

// Model is the help overlay.
type Model struct {
	Visible bool
	View    string // current view context
	Width   int
	Height  int
}

// New creates a help model.
func New() Model {
	return Model{}
}

// Update handles input.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, shared.Keys.Help) || key.Matches(msg, shared.Keys.Back) {
			m.Visible = false
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
	}
	return m, nil
}

// Render returns the help overlay content.
func (m Model) Render() string {
	title := shared.StyleModalTitle.Render("Keyboard Shortcuts")

	sections := []struct {
		name  string
		binds []string
	}{
		{
			name: "Global",
			binds: []string{
				"q / ctrl+c   quit",
				"?            toggle help",
				"C            switch cloud",
			},
		},
		{
			name: "Server List",
			binds: []string{
				"↑/k ↓/j      navigate",
				"enter         view detail",
				"c             create server",
				"d             delete server",
				"r             soft reboot",
				"R             force refresh",
				"/             filter",
			},
		},
		{
			name: "Server Detail",
			binds: []string{
				"↑/k ↓/j      scroll",
				"d             delete server",
				"r             soft reboot",
				"R             hard reboot",
				"esc           back to list",
			},
		},
		{
			name: "Create Form",
			binds: []string{
				"tab           next field",
				"shift+tab     prev field",
				"enter         open picker",
				"ctrl+s        submit",
				"esc           cancel",
			},
		},
		{
			name: "Modals",
			binds: []string{
				"y             confirm",
				"n / esc       cancel",
			},
		},
	}

	var b strings.Builder
	for _, s := range sections {
		b.WriteString(lipgloss.NewStyle().
			Bold(true).
			Foreground(shared.ColorSecondary).
			Render(s.name) + "\n")
		for _, bind := range s.binds {
			b.WriteString("  " + bind + "\n")
		}
		b.WriteString("\n")
	}

	content := title + "\n\n" + b.String() +
		shared.StyleHelp.Render("Press ? or esc to close")

	box := shared.StyleModal.Width(50).Render(content)

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
}
