package vmpassword

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/larkly/lazystack/internal/shared"
)

// Model is the admin password display modal.
type Model struct {
	Active     bool
	serverName string
	keyName    string
	keyPath    string
	plain      string
	encrypted  string
	note       string
	status     string
	width      int
	height     int

	// Credentials mode (NewCredentials): transient secrets, masked until
	// the user explicitly reveals them.
	title      string
	creds      []Credential
	credCursor int
	revealed   bool
}

// Credential is a transient secret for one server, such as the admin
// password returned by a rescue or evacuation. Its String and GoString
// never include the secret, so accidental formatting cannot leak it.
type Credential struct {
	Server string
	Secret string
}

// String implements fmt.Stringer without revealing the secret.
func (c Credential) String() string { return c.Server + ": " + masked }

// GoString implements fmt.GoStringer without revealing the secret.
func (c Credential) GoString() string {
	return fmt.Sprintf("vmpassword.Credential{Server:%q, Secret:<redacted>}", c.Server)
}

const masked = "********"

// NewCredentials creates a modal that lists transient per-server secrets.
// Secrets stay masked until the user presses r; c copies the selected one.
func NewCredentials(title, note string, creds []Credential) Model {
	shared.Debugf("[vmpassword] credentials modal %q with %d entries", title, len(creds))
	return Model{
		Active: true,
		title:  title,
		note:   note,
		creds:  creds,
	}
}

// New creates a password modal. Plain is the decrypted password (may be
// empty); encrypted is the base64 blob from Nova (may be empty, which means
// the server has no generated password). Note is a user-facing explanation
// of any degraded state.
func New(serverName, keyName, keyPath, plain, encrypted, note string) Model {
	shared.Debugf("[vmpassword] Init() server=%q hasPlain=%t hasEncrypted=%t", serverName, plain != "", encrypted != "")
	return Model{
		Active:     true,
		serverName: serverName,
		keyName:    keyName,
		keyPath:    keyPath,
		plain:      plain,
		encrypted:  encrypted,
		note:       note,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return nil
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
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.creds != nil {
		switch {
		case key.Matches(msg, shared.Keys.Up):
			if m.credCursor > 0 {
				m.credCursor--
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Down):
			if m.credCursor < len(m.creds)-1 {
				m.credCursor++
			}
			return m, nil
		case msg.String() == "r":
			m.revealed = !m.revealed
			return m, nil
		}
	}
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.Active = false
		return m, nil
	case msg.String() == "c":
		return m.copyToClipboard()
	default:
		return m, nil
	}
}

func (m Model) copyToClipboard() (Model, tea.Cmd) {
	value, label := m.copyValue()
	if value == "" {
		m.status = "Nothing to copy"
		return m, nil
	}
	if err := clipboard.WriteAll(value); err != nil {
		m.status = "Clipboard error: " + err.Error()
	} else {
		m.status = "Copied " + label + " to clipboard"
	}
	return m, nil
}

func (m Model) copyValue() (string, string) {
	if m.creds != nil {
		if m.credCursor >= 0 && m.credCursor < len(m.creds) {
			c := m.creds[m.credCursor]
			return c.Secret, "password for " + c.Server
		}
		return "", ""
	}
	if m.plain != "" {
		return m.plain, "password"
	}
	if m.encrypted != "" {
		return m.encrypted, "encrypted blob"
	}
	return "", ""
}

// View renders the password modal.
func (m Model) View() string {
	if m.creds != nil {
		return m.credentialsView()
	}
	title := shared.StyleModalTitle.Render("Admin Password")

	var body strings.Builder
	label := lipgloss.NewStyle().Foreground(shared.ColorSecondary)
	muted := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	value := lipgloss.NewStyle().Foreground(shared.ColorPrimary)

	body.WriteString("  " + label.Render("Server") + "  " + m.serverName + "\n")
	if m.keyName != "" {
		body.WriteString("  " + label.Render("Keypair") + " " + m.keyName)
		if m.keyPath != "" {
			body.WriteString(muted.Render("  (" + m.keyPath + ")"))
		}
		body.WriteString("\n")
	}
	body.WriteString("\n")

	switch {
	case m.plain != "":
		body.WriteString("  " + value.Render(m.plain) + "\n\n")
	case m.encrypted != "":
		body.WriteString("  " + label.Render("Encrypted (base64)") + "\n")
		body.WriteString("  " + muted.Render(truncate(m.encrypted, 60)) + "\n\n")
	default:
		body.WriteString("  " + muted.Render("No password set — this is likely not a Windows instance,") + "\n")
		body.WriteString("  " + muted.Render("or the password has not been generated yet.") + "\n\n")
	}

	if m.note != "" {
		body.WriteString("  " + muted.Render(m.note) + "\n\n")
	}
	if m.status != "" {
		body.WriteString("  " + lipgloss.NewStyle().Foreground(shared.ColorPrimary).Render(m.status) + "\n\n")
	}

	help := "  esc: close"
	if m.plain != "" || m.encrypted != "" {
		help = "  c: copy  esc: close"
	}
	body.WriteString(shared.StyleHelp.Render(help))

	content := title + "\n\n" + body.String()
	return m.renderModal(content)
}

func (m Model) credentialsView() string {
	title := shared.StyleModalTitle.Render(m.title)
	var body strings.Builder
	label := lipgloss.NewStyle().Foreground(shared.ColorSecondary)
	muted := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	value := lipgloss.NewStyle().Foreground(shared.ColorPrimary)
	for i, c := range m.creds {
		prefix := "  "
		if i == m.credCursor {
			prefix = "▸ "
		}
		secret := muted.Render(masked)
		if m.revealed {
			secret = value.Render(c.Secret)
		}
		body.WriteString(prefix + label.Render(c.Server) + "  " + secret + "\n")
	}
	body.WriteString("\n")
	if m.note != "" {
		body.WriteString("  " + muted.Render(m.note) + "\n\n")
	}
	if m.status != "" {
		body.WriteString("  " + value.Render(m.status) + "\n\n")
	}
	reveal := "r: reveal"
	if m.revealed {
		reveal = "r: hide"
	}
	help := "  " + reveal + "  c: copy  esc: close"
	if len(m.creds) > 1 {
		help = "  ↑↓: select  " + reveal + "  c: copy  esc: close"
	}
	body.WriteString(shared.StyleHelp.Render(help))
	return m.renderModal(title + "\n\n" + body.String())
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func (m Model) renderModal(content string) string {
	modalWidth := 64
	if m.width > 0 && m.width < 72 {
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
