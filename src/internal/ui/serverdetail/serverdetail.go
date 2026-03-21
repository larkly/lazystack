package serverdetail

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bosse/lazystack/internal/shared"
	"github.com/bosse/lazystack/internal/compute"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type serverDetailLoadedMsg struct {
	server *compute.Server
}

type serverDetailErrMsg struct {
	err error
}

type detailTickMsg struct{}

// Model is the server detail view.
type Model struct {
	client          *gophercloud.ServiceClient
	serverID        string
	server          *compute.Server
	loading         bool
	spinner         spinner.Model
	width           int
	height          int
	scroll          int
	err             string
	refreshInterval time.Duration
	pendingAction   string // e.g. "Resize confirmed" — shown until server state catches up
}

// New creates a server detail model.
func New(client *gophercloud.ServiceClient, serverID string, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		client:          client,
		serverID:        serverID,
		loading:         true,
		spinner:         s,
		refreshInterval: refreshInterval,
	}
}

// Init fetches the server details.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchServer(), m.tickCmd())
}

// ServerID returns the current server ID.
func (m Model) ServerID() string {
	return m.serverID
}

// ServerName returns the current server name.
func (m Model) ServerName() string {
	if m.server != nil {
		return m.server.Name
	}
	return m.serverID
}

// ServerStatus returns the current server status.
func (m Model) ServerStatus() string {
	if m.server != nil {
		return m.server.Status
	}
	return ""
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case serverDetailLoadedMsg:
		m.loading = false
		// Clear pending action if server state has changed
		if m.pendingAction != "" && msg.server != nil {
			if msg.server.Status != "VERIFY_RESIZE" {
				m.pendingAction = ""
			}
		}
		m.server = msg.server
		m.err = ""
		return m, nil

	case serverDetailErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case detailTickMsg:
		return m, tea.Batch(m.fetchServer(), m.tickCmd())

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Back):
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "serverlist"}
			}
		case key.Matches(msg, shared.Keys.Up):
			if m.scroll > 0 {
				m.scroll--
			}
		case key.Matches(msg, shared.Keys.Down):
			m.scroll++
		case key.Matches(msg, shared.Keys.PageDown):
			m.scroll += m.height - 5
		case key.Matches(msg, shared.Keys.PageUp):
			m.scroll -= m.height - 5
			if m.scroll < 0 {
				m.scroll = 0
			}
		case key.Matches(msg, shared.Keys.Delete):
			// Handled by root model
		case key.Matches(msg, shared.Keys.Reboot):
			// Handled by root model
		}
	}
	return m, nil
}

// View renders the server detail.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Server Detail")
	if m.loading {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if m.server == nil {
		return b.String()
	}

	s := m.server

	// Show resize-related banners
	if m.pendingAction != "" {
		banner := lipgloss.NewStyle().
			Foreground(shared.ColorSuccess).
			Bold(true).
			Render(fmt.Sprintf("  ✓ %s — waiting for server...", m.pendingAction))
		b.WriteString(banner + "\n\n")
	} else if s.Status == "VERIFY_RESIZE" {
		banner := lipgloss.NewStyle().
			Foreground(shared.ColorWarning).
			Bold(true).
			Render("  ⚠ Resize pending — ctrl+y confirm • ctrl+x revert")
		b.WriteString(banner + "\n\n")
	}

	locked := ""
	if s.Locked {
		locked = "yes"
	}

	props := []struct {
		label string
		value string
	}{
		{"Name", s.Name},
		{"ID", s.ID},
		{"Status", s.Status},
		{"Power State", s.PowerState},
		{"Flavor", s.FlavorName},
		{"Image", s.ImageName},
		{"Image ID", s.ImageID},
		{"Key Pair", s.KeyName},
		{"Locked", locked},
		{"Tenant ID", s.TenantID},
		{"Availability Zone", s.AZ},
		{"Created", s.Created.Format("2006-01-02 15:04:05")},
		{"Security Groups", strings.Join(s.SecGroups, ", ")},
		{"Volumes", strings.Join(s.VolAttach, ", ")},
	}

	lines := make([]string, 0, len(props)+len(s.Networks))
	for _, p := range props {
		if p.value == "" {
			continue
		}
		label := shared.StyleLabel.Render(p.label)
		value := shared.StyleValue.Render(p.value)
		if p.label == "Status" {
			value = StatusStyle(p.value).Render(p.value)
		}
		lines = append(lines, fmt.Sprintf("  %s %s", label, value))
	}

	// Networks section
	if len(s.Networks) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("  %s", shared.StyleLabel.Render("Networks")))
		netNames := make([]string, 0, len(s.Networks))
		for name := range s.Networks {
			netNames = append(netNames, name)
		}
		sort.Strings(netNames)
		for _, name := range netNames {
			ips := s.Networks[name]
			lines = append(lines, fmt.Sprintf("    %s  %s",
				lipgloss.NewStyle().Foreground(shared.ColorSecondary).Render(name),
				shared.StyleValue.Render(strings.Join(ips, ", "))))
		}
	}

	// Apply scroll
	viewHeight := m.height - 5
	if s.Status == "VERIFY_RESIZE" {
		viewHeight -= 2
	}
	if viewHeight < 1 {
		viewHeight = 1
	}
	if m.scroll > len(lines)-viewHeight {
		m.scroll = max(0, len(lines)-viewHeight)
	}

	end := m.scroll + viewHeight
	if end > len(lines) {
		end = len(lines)
	}

	for _, line := range lines[m.scroll:end] {
		b.WriteString(line + "\n")
	}

	return b.String()
}

// StatusStyle returns the style for a server status.
func StatusStyle(status string) lipgloss.Style {
	color, ok := shared.StatusColors[status]
	if !ok {
		color = shared.ColorFg
	}
	return lipgloss.NewStyle().Foreground(color)
}

func (m Model) fetchServer() tea.Cmd {
	client := m.client
	id := m.serverID
	return func() tea.Msg {
		srv, err := compute.GetServer(context.Background(), client, id)
		if err != nil {
			return serverDetailErrMsg{err: err}
		}
		return serverDetailLoadedMsg{server: srv}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return detailTickMsg{}
	})
}

// ForceRefresh triggers a manual reload of the server detail.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchServer())
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// ServerFlavor returns the current server flavor name.
func (m Model) ServerFlavor() string {
	if m.server != nil {
		return m.server.FlavorName
	}
	return ""
}

// SetServer updates the server data directly.
func (m *Model) SetServer(s *compute.Server) {
	if m.pendingAction != "" && s != nil && s.Status != "VERIFY_RESIZE" {
		m.pendingAction = ""
	}
	m.server = s
	m.loading = false
	m.err = ""
}

// SetPendingAction marks an action as in-progress. The banner stays
// until the server's real status changes away from the old state.
func (m *Model) SetPendingAction(action string) {
	m.pendingAction = action
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	if m.pendingAction != "" {
		return "↑↓ scroll • esc back • ? help"
	}
	if m.server != nil && m.server.Status == "VERIFY_RESIZE" {
		return "^y confirm resize • ^x revert resize • ↑↓ scroll • ^d delete • esc back • ? help"
	}
	return "↑↓ scroll • ^d delete • ^a assign FIP • ^o reboot • R refresh • esc back • ? help"
}
