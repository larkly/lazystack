package modal

import (
	"github.com/bosse/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ErrorDismissedMsg is sent when the error modal is dismissed.
type ErrorDismissedMsg struct{}

// ErrorModel is an error display modal.
type ErrorModel struct {
	Context string
	Err     string
	Width   int
	Height  int
}

// NewError creates an error modal.
func NewError(context string, err error) ErrorModel {
	return ErrorModel{
		Context: context,
		Err:     err.Error(),
	}
}

// Update handles input for the error modal.
func (m ErrorModel) Update(msg tea.Msg) (ErrorModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Enter),
			key.Matches(msg, shared.Keys.Back):
			return m, func() tea.Msg {
				return ErrorDismissedMsg{}
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
		Render(m.Err)

	btnStyle := lipgloss.NewStyle().
		Padding(0, 3).
		Background(shared.ColorPrimary).
		Foreground(shared.ColorBg).
		Bold(true)
	button := btnStyle.Render("[enter] OK")

	content := title + "\n\n" + body + "\n\n" + button
	box := shared.StyleErrorModal.Width(60).Render(content)

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
}

// SetSize updates dimensions.
func (m *ErrorModel) SetSize(w, h int) {
	m.Width = w
	m.Height = h
}
