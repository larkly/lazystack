package modal

import (
	"fmt"

	"github.com/bosse/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ConfirmAction is the result of a confirmation dialog.
type ConfirmAction struct {
	Action   string
	ServerID string
	Name     string
	Confirm  bool
}

// ConfirmModel is a confirmation dialog.
type ConfirmModel struct {
	Action   string
	ServerID string
	Name     string
	Width    int
	Height   int
	focused  int // 0 = confirm, 1 = cancel
}

// NewConfirm creates a confirmation dialog.
func NewConfirm(action, serverID, name string) ConfirmModel {
	return ConfirmModel{
		Action:   action,
		ServerID: serverID,
		Name:     name,
		focused:  1, // default to cancel for safety
	}
}

func (m ConfirmModel) confirmMsg() tea.Cmd {
	return func() tea.Msg {
		return ConfirmAction{
			Action:   m.Action,
			ServerID: m.ServerID,
			Name:     m.Name,
			Confirm:  true,
		}
	}
}

func (m ConfirmModel) cancelMsg() tea.Cmd {
	return func() tea.Msg {
		return ConfirmAction{
			Action:   m.Action,
			ServerID: m.ServerID,
			Name:     m.Name,
			Confirm:  false,
		}
	}
}

// Update handles input for the confirmation dialog.
func (m ConfirmModel) Update(msg tea.Msg) (ConfirmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Confirm):
			return m, m.confirmMsg()
		case key.Matches(msg, shared.Keys.Deny):
			return m, m.cancelMsg()
		case key.Matches(msg, shared.Keys.Back):
			return m, m.cancelMsg()
		case key.Matches(msg, shared.Keys.Tab),
			key.Matches(msg, shared.Keys.ShiftTab),
			key.Matches(msg, shared.Keys.Left),
			key.Matches(msg, shared.Keys.Right),
			key.Matches(msg, shared.Keys.Up),
			key.Matches(msg, shared.Keys.Down):
			m.focused = 1 - m.focused
		case key.Matches(msg, shared.Keys.Enter):
			if m.focused == 0 {
				return m, m.confirmMsg()
			}
			return m, m.cancelMsg()
		}
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
	}
	return m, nil
}

// View renders the confirmation dialog.
func (m ConfirmModel) View() string {
	title := shared.StyleModalTitle.Render(fmt.Sprintf("Confirm %s", m.Action))

	body := fmt.Sprintf("Are you sure you want to %s server %q?",
		m.Action, m.Name)

	// Buttons
	confirmStyle := lipgloss.NewStyle().Padding(0, 2).Background(shared.ColorMuted).Foreground(shared.ColorFg)
	cancelStyle := lipgloss.NewStyle().Padding(0, 2).Background(shared.ColorMuted).Foreground(shared.ColorFg)
	if m.focused == 0 {
		confirmStyle = confirmStyle.Background(shared.ColorSuccess).Foreground(shared.ColorBg).Bold(true)
	} else {
		cancelStyle = cancelStyle.Background(shared.ColorError).Foreground(shared.ColorBg).Bold(true)
	}
	buttons := confirmStyle.Render("[y] Confirm") + "  " + cancelStyle.Render("[n] Cancel")

	content := title + "\n\n" + body + "\n\n" + buttons
	box := shared.StyleModal.Width(50).Render(content)

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
}

// SetSize updates dimensions.
func (m *ConfirmModel) SetSize(w, h int) {
	m.Width = w
	m.Height = h
}
