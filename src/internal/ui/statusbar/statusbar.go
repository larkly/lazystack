package statusbar

import (
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"charm.land/lipgloss/v2"
)

// Model holds the status bar state.
type Model struct {
	CloudName   string
	ProjectName string
	Region      string
	CurrentView string
	Width       int
	Hint        string
	Error       string
	Version     string
}

// New creates a new status bar.
func New(version string) Model {
	return Model{
		CurrentView: "cloudpicker",
		Hint:        "Select a cloud to connect",
		Version:     version,
	}
}

// Render renders the status bar.
func (m Model) Render() string {
	style := shared.StyleStatusBar

	left := ""
	if m.CloudName != "" {
		if m.ProjectName != "" {
			left = fmt.Sprintf(" %s %s  %s %s  %s %s",
				shared.StyleStatusBarKey.Render("cloud:"),
				m.CloudName,
				shared.StyleStatusBarKey.Render("project:"),
				m.ProjectName,
				shared.StyleStatusBarKey.Render("region:"),
				m.Region,
			)
		} else {
			left = fmt.Sprintf(" %s %s  %s %s",
				shared.StyleStatusBarKey.Render("cloud:"),
				m.CloudName,
				shared.StyleStatusBarKey.Render("region:"),
				m.Region,
			)
		}
	}

	right := ""
	if m.Error != "" {
		right = lipgloss.NewStyle().
			Foreground(shared.ColorError).
			Render(fmt.Sprintf(" ⚠ %s ", m.Error))
	} else if m.Hint != "" {
		right = shared.StyleHelp.Render(fmt.Sprintf(" %s ", m.Hint))
	}

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := m.Width - leftW - rightW
	if gap < 0 {
		right = ""
		gap = m.Width - leftW
		if gap < 0 {
			gap = 0
		}
	}

	bar := left + strings.Repeat(" ", gap) + right

	return style.Render(bar)
}
