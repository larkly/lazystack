package consoleurl

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Model is the console URL display modal.
type Model struct {
	Active     bool
	url        string
	serverName string
	status     string
	width      int
	height     int
}

// New creates a console URL modal.
func New(url, serverName string) Model {
	shared.Debugf("[consoleurl] Init() server=%q url=%q", serverName, url)
	return Model{
		Active:     true,
		url:        url,
		serverName: serverName,
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
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.Active = false
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		return m.openInBrowser()
	case msg.String() == "c":
		return m.copyToClipboard()
	default:
		return m, nil
	}
}

func (m Model) openInBrowser() (Model, tea.Cmd) {
	url := m.url
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		m.status = "Failed to open browser: " + err.Error()
	} else {
		m.status = "Opened in browser"
	}
	return m, nil
}

func (m Model) copyToClipboard() (Model, tea.Cmd) {
	if err := clipboard.WriteAll(m.url); err != nil {
		m.status = "Clipboard error: " + err.Error()
	} else {
		m.status = "Copied to clipboard"
	}
	return m, nil
}

// View renders the console URL modal.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Console URL")

	var body strings.Builder

	label := lipgloss.NewStyle().Foreground(shared.ColorSecondary)
	muted := lipgloss.NewStyle().Foreground(shared.ColorMuted)

	body.WriteString("  " + label.Render("Server") + "  " + m.serverName + "\n\n")

	// Truncate URL for display if needed
	urlDisplay := m.url
	maxURLWidth := 44
	if len(urlDisplay) > maxURLWidth {
		urlDisplay = urlDisplay[:maxURLWidth-3] + "..."
	}
	body.WriteString("  " + muted.Render(urlDisplay) + "\n\n")

	if m.status != "" {
		body.WriteString("  " + lipgloss.NewStyle().Foreground(shared.ColorPrimary).Render(m.status) + "\n\n")
	}

	body.WriteString(shared.StyleHelp.Render("  enter: open in browser  c: copy  esc: close"))

	content := title + "\n\n" + body.String()
	return m.renderModal(content)
}

func (m Model) renderModal(content string) string {
	modalWidth := 54
	if m.width > 0 && m.width < 64 {
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
