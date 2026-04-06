package modal

import (
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ErrorDismissedMsg is sent when the error modal is dismissed.
type ErrorDismissedMsg struct{}

// ErrorModel is an error display modal.
type ErrorModel struct {
	Context        string
	FriendlyError  string // User-friendly message
	RawError       string // Full raw error (expandable)
	ShowDetails    bool   // Whether details are expanded
	HTTPStatusCode int    // For display
	Width          int
	Height         int
}

// NewError creates an error modal.
func NewError(context string, err error) ErrorModel {
	shared.Debugf("[error] shown context=%s err=%v", context, err)
	parsed := shared.ParseError(err)
	return ErrorModel{
		Context:       context,
		FriendlyError: parsed.FriendlyMessage,
		RawError:      parsed.RawError,
		HTTPStatusCode: parsed.HTTPStatusCode,
		ShowDetails:   false,
	}
}

// Update handles input for the error modal.
func (m ErrorModel) Update(msg tea.Msg) (ErrorModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Enter),
			key.Matches(msg, shared.Keys.Back):
			shared.Debugf("[error] dismissed context=%s", m.Context)
			return m, func() tea.Msg {
				return ErrorDismissedMsg{}
			}
		default:
			if strings.ToLower(msg.String()) == "d" {
				m.ShowDetails = !m.ShowDetails
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
	}
	return m, nil
}

// View renders the error modal.
func (m ErrorModel) View() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(shared.ColorError).
		MarginBottom(1).
		Render("Error: " + m.Context)

	body := lipgloss.NewStyle().
		Foreground(shared.ColorFg).
		Render(m.FriendlyError)

	// Build details section if expanded.
	details := ""
	if m.ShowDetails {
		detailStyle := lipgloss.NewStyle().
			Foreground(shared.ColorMuted).
			MarginTop(1).
			Padding(0, 1)
		details = detailStyle.Render("Raw: " + m.RawError)
	}

	btnStyle := lipgloss.NewStyle().
		Padding(0, 3).
		Background(shared.ColorPrimary).
		Foreground(shared.ColorBg).
		Bold(true)
	button := btnStyle.Render("[enter] OK")

	detailLabel := "[d] Details"
	if m.ShowDetails {
		detailLabel = "[d] Hide"
	}
	help := detailLabel

	content := title + "\n\n" + body + details + "\n\n" + button + "  " + help
	box := shared.StyleErrorModal.Width(60).Render(content)

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
}

// SetSize updates dimensions.
func (m *ErrorModel) SetSize(w, h int) {
	m.Width = w
	m.Height = h
}
