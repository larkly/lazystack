package statusbar

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/larkly/lazystack/internal/shared"
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
	StickyHint  string // survives until next key press
}

// New creates a new status bar.
func New(version string) Model {
	return Model{
		CurrentView: "cloudpicker",
		Hint:        "Select a cloud to connect",
		Version:     version,
	}
}

// minHintCells is the narrowest space a truncated hint is still shown in.
const minHintCells = 10

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
			Render(fmt.Sprintf(" %s %s ", shared.Glyph("⚠", "!"), m.Error))
	} else if m.StickyHint != "" {
		right = shared.StyleHelp.Render(fmt.Sprintf(" %s ", m.StickyHint))
	} else if m.Hint != "" {
		right = shared.StyleHelp.Render(fmt.Sprintf(" %s ", m.Hint))
	}

	leftW := lipgloss.Width(left)
	if avail := m.Width - leftW; lipgloss.Width(right) > avail {
		// Shorten the hint to the space left of the cloud info; drop it
		// only when too little would remain to be useful.
		if avail >= minHintCells {
			right = shared.TruncateCells(right, avail)
		} else {
			right = ""
		}
	}
	gap := max(0, m.Width-leftW-lipgloss.Width(right))

	bar := left + strings.Repeat(" ", gap) + right

	return style.Render(bar)
}
