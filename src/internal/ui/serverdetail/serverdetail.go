package serverdetail

import (
	"context"
	"fmt"
	"strings"

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

// Model is the server detail view.
type Model struct {
	client   *gophercloud.ServiceClient
	serverID string
	server   *compute.Server
	loading  bool
	spinner  spinner.Model
	width    int
	height   int
	scroll   int
	err      string
}

// New creates a server detail model.
func New(client *gophercloud.ServiceClient, serverID string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		client:   client,
		serverID: serverID,
		loading:  true,
		spinner:  s,
	}
}

// Init fetches the server details.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchServer())
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

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case serverDetailLoadedMsg:
		m.loading = false
		m.server = msg.server
		m.err = ""
		return m, nil

	case serverDetailErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

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
	props := []struct {
		label string
		value string
	}{
		{"Name", s.Name},
		{"ID", s.ID},
		{"Status", s.Status},
		{"Flavor", s.FlavorID},
		{"Image", s.ImageID},
		{"IP", s.IP},
		{"Key Pair", s.KeyName},
		{"Tenant ID", s.TenantID},
		{"Availability Zone", s.AZ},
		{"Created", s.Created.Format("2006-01-02 15:04:05")},
		{"Security Groups", strings.Join(s.SecGroups, ", ")},
		{"Volumes", strings.Join(s.VolAttach, ", ")},
	}

	lines := make([]string, 0, len(props))
	for _, p := range props {
		label := shared.StyleLabel.Render(p.label)
		value := shared.StyleValue.Render(p.value)
		if p.label == "Status" {
			value = StatusStyle(p.value).Render(p.value)
		}
		lines = append(lines, fmt.Sprintf("  %s %s", label, value))
	}

	// Apply scroll
	viewHeight := m.height - 5
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

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	return "↑↓ scroll • d delete • r reboot • R hard reboot • esc back • ? help"
}
