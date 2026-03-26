package serverpicker

import (
	"context"
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/volume"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type serversLoadedMsg struct{ servers []compute.Server }
type fetchErrMsg struct{ err error }
type attachDoneMsg struct{ serverName, volumeName string }
type attachErrMsg struct{ err error }

// Model is the server picker modal for volume attach.
type Model struct {
	Active        bool
	computeClient *gophercloud.ServiceClient
	volumeID      string
	volumeName    string
	servers       []compute.Server
	filtered      []compute.Server
	cursor        int
	loading       bool
	submitting    bool
	spinner       spinner.Model
	filter        string
	width         int
	height        int
	err           string
	scrollOff     int
}

// New creates a server picker for attaching a volume.
func New(computeClient *gophercloud.ServiceClient, volumeID, volumeName string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		Active:        true,
		computeClient: computeClient,
		volumeID:      volumeID,
		volumeName:    volumeName,
		loading:       true,
		spinner:       s,
	}
}

// Init fetches available servers.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchServers())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case serversLoadedMsg:
		m.loading = false
		m.servers = msg.servers
		m.applyFilter()
		return m, nil

	case fetchErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case attachDoneMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{
				Action: "Attached",
				Name:   fmt.Sprintf("%s → %s", m.volumeName, msg.serverName),
			}
		}

	case attachErrMsg:
		m.submitting = false
		m.err = msg.err.Error()
		return m, nil

	case spinner.TickMsg:
		if m.loading || m.submitting {
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
		if m.loading || m.submitting {
			return m, nil
		}
		switch {
		case key.Matches(msg, shared.Keys.Back):
			m.Active = false
			return m, nil
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.Enter):
			if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
				srv := m.filtered[m.cursor]
				m.submitting = true
				return m, tea.Batch(m.spinner.Tick, m.attachVolume(srv))
			}
		default:
			// Handle filter typing
			s := msg.String()
			if s == "backspace" {
				if len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
					m.cursor = 0
					m.scrollOff = 0
					m.applyFilter()
				}
			} else if len(s) == 1 && s[0] >= 32 && s[0] < 127 {
				m.filter += s
				m.cursor = 0
				m.scrollOff = 0
				m.applyFilter()
			}
		}
	}
	return m, nil
}

func (m *Model) applyFilter() {
	if m.filter == "" {
		m.filtered = m.servers
		return
	}
	q := strings.ToLower(m.filter)
	m.filtered = nil
	for _, srv := range m.servers {
		if strings.Contains(strings.ToLower(srv.Name), q) ||
			strings.Contains(strings.ToLower(srv.ID), q) {
			m.filtered = append(m.filtered, srv)
		}
	}
}

func (m *Model) ensureVisible() {
	th := m.listHeight()
	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+th {
		m.scrollOff = m.cursor - th + 1
	}
}

func (m Model) listHeight() int {
	h := m.height - 14
	if h < 3 {
		h = 3
	}
	return h
}

// View renders the server picker modal.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Attach " + m.volumeName + " to Server")

	var body string
	if m.loading {
		body = m.spinner.View() + " Loading servers..."
	} else if m.submitting {
		body = m.spinner.View() + " Attaching..."
	} else if m.err != "" {
		body = lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.err)
		body += "\n\n" + shared.StyleHelp.Render("esc to close")
	} else if len(m.filtered) == 0 {
		body = shared.StyleHelp.Render("No servers found")
		body += "\n\n" + shared.StyleHelp.Render("esc to close")
	} else {
		var lines []string
		th := m.listHeight()
		end := m.scrollOff + th
		if end > len(m.filtered) {
			end = len(m.filtered)
		}

		for i := m.scrollOff; i < end; i++ {
			srv := m.filtered[i]
			cursor := "  "
			if i == m.cursor {
				cursor = "▸ "
			}
			style := lipgloss.NewStyle().Foreground(shared.ColorFg)
			if i == m.cursor {
				style = style.Foreground(shared.ColorHighlight).Bold(true)
			}
			statusStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
			if srv.Status == "ACTIVE" {
				statusStyle = statusStyle.Foreground(shared.ColorSuccess)
			}
			line := cursor + style.Render(srv.Name) + " " + statusStyle.Render(srv.Status)
			lines = append(lines, line)
		}
		body = strings.Join(lines, "\n")

		filterHint := ""
		if m.filter != "" {
			filterHint = fmt.Sprintf("\n\nFilter: %s", m.filter)
		}
		body += filterHint
		body += "\n\n" + shared.StyleHelp.Render("↑↓ navigate • enter select • type to filter • esc cancel")
	}

	content := title + "\n\n" + body
	modalWidth := 60
	if m.width > 0 && m.width < 70 {
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

func (m Model) fetchServers() tea.Cmd {
	client := m.computeClient
	return func() tea.Msg {
		servers, err := compute.ListServers(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		// Filter to ACTIVE servers only
		var active []compute.Server
		for _, s := range servers {
			if s.Status == "ACTIVE" || s.Status == "SHUTOFF" {
				active = append(active, s)
			}
		}
		return serversLoadedMsg{servers: active}
	}
}

func (m Model) attachVolume(srv compute.Server) tea.Cmd {
	client := m.computeClient
	volumeID := m.volumeID
	volumeName := m.volumeName
	serverID := srv.ID
	serverName := srv.Name
	_ = volumeName // used in the msg struct
	return func() tea.Msg {
		err := volume.AttachVolume(context.Background(), client, serverID, volumeID)
		if err != nil {
			return attachErrMsg{err: err}
		}
		return attachDoneMsg{serverName: serverName, volumeName: volumeName}
	}
}
