package sshprompt

import (
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// SSHConnectMsg is emitted when the user confirms the SSH username.
type SSHConnectMsg struct {
	User    string
	IP      string
	KeyPath string
}

// Model is the SSH username prompt overlay modal.
type Model struct {
	Active     bool
	serverName string
	ip         string
	keyPath    string
	input      textinput.Model
	err        string
	width      int
	height     int
}

// New creates an SSH prompt modal for the given server.
func New(serverName, ip, keyPath string) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "username"
	ti.CharLimit = 64
	ti.SetWidth(30)
	ti.Focus()

	return Model{
		Active:     true,
		serverName: serverName,
		ip:         ip,
		keyPath:    keyPath,
		input:      ti,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.Active = false
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		return m.submit()
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	user := strings.TrimSpace(m.input.Value())
	if user == "" {
		m.err = "Username cannot be empty"
		return m, nil
	}
	m.Active = false
	return m, func() tea.Msg {
		return SSHConnectMsg{
			User:    user,
			IP:      m.ip,
			KeyPath: m.keyPath,
		}
	}
}

// View renders the SSH prompt overlay.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("SSH into Server")

	var body strings.Builder

	if m.err != "" {
		body.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  "+m.err) + "\n\n")
	}

	muted := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	label := lipgloss.NewStyle().Foreground(shared.ColorSecondary)

	body.WriteString("  " + label.Render("Server") + "  " + m.serverName + "\n")
	body.WriteString("  " + label.Render("Host  ") + "  " + m.ip + "\n")
	if m.keyPath != "" {
		body.WriteString("  " + label.Render("Key   ") + "  " + muted.Render(m.keyPath) + "\n")
	} else {
		body.WriteString("  " + label.Render("Key   ") + "  " + muted.Render("(default)") + "\n")
	}
	body.WriteString("\n")
	body.WriteString("  " + label.Render("User  ") + "  " + m.input.View() + "\n\n")
	body.WriteString(shared.StyleHelp.Render("  enter: connect  esc: cancel"))

	content := title + "\n\n" + body.String()
	return m.renderModal(content)
}

func (m Model) renderModal(content string) string {
	modalWidth := 50
	if m.width > 0 && m.width < 60 {
		modalWidth = m.width - 6
	}
	box := shared.StyleModal.Width(modalWidth).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}
