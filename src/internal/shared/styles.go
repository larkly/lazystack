package shared

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

var (
	ColorPrimary   = lipgloss.Color("#7D56F4")
	ColorSecondary = lipgloss.Color("#6C71C4")
	ColorSuccess   = lipgloss.Color("#2AA198")
	ColorWarning   = lipgloss.Color("#B58900")
	ColorError     = lipgloss.Color("#DC322F")
	ColorMuted     = lipgloss.Color("#586E75")
	ColorBg        = lipgloss.Color("#002B36")
	ColorFg        = lipgloss.Color("#839496")
	ColorHighlight = lipgloss.Color("#FDF6E3")
	ColorCyan      = lipgloss.Color("#2AA198")

	// Status colors for server states.
	StatusColors = map[string]color.Color{
		"ACTIVE":            ColorSuccess,
		"BUILD":             ColorWarning,
		"SHUTOFF":           lipgloss.Color("#586E75"),
		"ERROR":             ColorError,
		"REBOOT":            ColorCyan,
		"HARD_REBOOT":       ColorCyan,
		"RESIZE":            ColorWarning,
		"VERIFY_RESIZE":     ColorWarning,
		"MIGRATING":         ColorWarning,
		"PAUSED":            ColorMuted,
		"SUSPENDED":         ColorMuted,
		"DELETED":           ColorError,
		"SOFT_DELETED":      ColorError,
		"SHELVED":           ColorMuted,
		"SHELVED_OFFLOADED": ColorMuted,
	}

	PowerColors = map[string]color.Color{
		"RUNNING":  ColorSuccess,
		"PAUSED":   ColorMuted,
		"SHUTDOWN": lipgloss.Color("#586E75"),
		"CRASHED":  ColorError,
		"SUSPENDED": ColorMuted,
		"NOSTATE":  ColorWarning,
	}

	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			PaddingLeft(1)

	StyleStatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("#073642")).
			Foreground(ColorFg).
			PaddingLeft(1).
			PaddingRight(1)

	StyleStatusBarKey = lipgloss.NewStyle().
				Background(lipgloss.Color("#073642")).
				Foreground(ColorPrimary).
				Bold(true)

	StyleHelp = lipgloss.NewStyle().
			Foreground(ColorMuted)

	StyleModal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2)

	StyleModalTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1)

	StyleErrorModal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorError).
			Padding(1, 2)

	StyleSelected = lipgloss.NewStyle().
			Background(lipgloss.Color("#073642")).
			Foreground(ColorHighlight)

	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSecondary)

	StyleLabel = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true).
			Width(20)

	StyleValue = lipgloss.NewStyle().
			Foreground(ColorFg)
)
