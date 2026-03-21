package statusbar

import (
	"fmt"

	"github.com/bosse/lazystack/internal/shared"
	"charm.land/lipgloss/v2"
)

// Model holds the status bar state.
type Model struct {
	CloudName   string
	Region      string
	CurrentView string
	Width       int
	Hint        string
	Error       string
}

// New creates a new status bar.
func New() Model {
	return Model{
		CurrentView: "cloudpicker",
		Hint:        "Select a cloud to connect",
	}
}

// Render renders the status bar.
func (m Model) Render() string {
	left := ""
	if m.CloudName != "" {
		left = fmt.Sprintf(" %s %s  %s %s",
			shared.StyleStatusBarKey.Render("cloud:"),
			m.CloudName,
			shared.StyleStatusBarKey.Render("region:"),
			m.Region,
		)
	}

	right := ""
	if m.Error != "" {
		right = lipgloss.NewStyle().
			Foreground(shared.ColorError).
			Render(fmt.Sprintf(" ⚠ %s ", m.Error))
	} else if m.Hint != "" {
		right = shared.StyleHelp.Render(fmt.Sprintf(" %s ", m.Hint))
	}

	gap := m.Width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}

	bar := left + lipgloss.NewStyle().Width(gap).Render("") + right

	return shared.StyleStatusBar.Width(m.Width).Render(bar)
}
