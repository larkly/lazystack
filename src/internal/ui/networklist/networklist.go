package networklist

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type networksLoadedMsg struct {
	networks []network.Network
	subnets  map[string]network.Subnet // ID → Subnet
}
type networksErrMsg struct{ err error }
type tickMsg struct{}

// Model is the network browser view.
type Model struct {
	client          *gophercloud.ServiceClient
	networks        []network.Network
	subnets         map[string]network.Subnet
	cursor          int
	expanded        map[int]bool
	scrollOff       int
	width           int
	height          int
	loading         bool
	spinner         spinner.Model
	err             string
	refreshInterval time.Duration
}

// New creates a network list model.
func New(client *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:          client,
		loading:         true,
		spinner:         s,
		expanded:        make(map[int]bool),
		subnets:         make(map[string]network.Subnet),
		refreshInterval: refreshInterval,
	}
}

// Init starts the initial fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchNetworks())
}

// ForceRefresh triggers a manual reload.
func (m Model) ForceRefresh() tea.Cmd {
	return m.fetchNetworks()
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case networksLoadedMsg:
		m.loading = false
		m.networks = msg.networks
		m.subnets = msg.subnets
		m.err = ""
		return m, nil
	case networksErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil
	case tickMsg:
		return m, m.fetchNetworks()
	case shared.TickMsg:
		return m, m.fetchNetworks()
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
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.ensureVisible()
		}
	case key.Matches(msg, shared.Keys.Down):
		if m.cursor < len(m.networks)-1 {
			m.cursor++
			m.ensureVisible()
		}
	case key.Matches(msg, shared.Keys.Enter):
		m.expanded[m.cursor] = !m.expanded[m.cursor]
	case key.Matches(msg, shared.Keys.PageDown):
		th := m.tableHeight()
		m.cursor += th
		if m.cursor >= len(m.networks) {
			m.cursor = len(m.networks) - 1
		}
		m.ensureVisible()
	case key.Matches(msg, shared.Keys.PageUp):
		th := m.tableHeight()
		m.cursor -= th
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureVisible()
	}
	return m, nil
}

func (m Model) tableHeight() int {
	h := m.height - 4
	if h < 3 {
		h = 3
	}
	return h
}

func (m *Model) ensureVisible() {
	th := m.tableHeight()
	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+th {
		m.scrollOff = m.cursor - th + 1
	}
}

// View renders the network list.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Networks")
	if m.loading && len(m.networks) == 0 {
		title += " " + m.spinner.View()
	}
	if m.err != "" {
		title += " " + lipgloss.NewStyle().Foreground(shared.ColorError).Render(m.err)
	}
	b.WriteString(title + "\n")

	if len(m.networks) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No networks found") + "\n")
		return b.String()
	}

	// Header
	nameW := 25
	statusW := 10
	subnetsW := 8
	sharedW := 8
	header := fmt.Sprintf("  %-*s %-*s %-*s %-*s",
		nameW, "Name", statusW, "Status", subnetsW, "Subnets", sharedW, "Shared")
	b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Render(header) + "\n")

	// Build visible lines
	type line struct {
		text string
	}
	var lines []line

	for i, net := range m.networks {
		cursor := "  "
		isCurrent := i == m.cursor
		if isCurrent {
			cursor = "▸ "
		}

		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if isCurrent {
			style = style.Background(lipgloss.Color("#073642"))
		}

		statusStyle := lipgloss.NewStyle().Foreground(shared.ColorSuccess)
		if net.Status != "ACTIVE" {
			statusStyle = statusStyle.Foreground(shared.ColorWarning)
		}

		sharedStr := "no"
		if net.Shared {
			sharedStr = "yes"
		}

		row := fmt.Sprintf("%s%-*s %s %-*d %-*s",
			cursor,
			nameW, truncate(net.Name, nameW),
			statusStyle.Width(statusW).Render(net.Status),
			subnetsW, len(net.SubnetIDs),
			sharedW, sharedStr,
		)
		if isCurrent {
			lines = append(lines, line{text: style.Render(row)})
		} else {
			lines = append(lines, line{text: style.Render(row)})
		}

		// Show expanded subnets
		if m.expanded[i] {
			for _, subID := range net.SubnetIDs {
				sub, ok := m.subnets[subID]

				subStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)

				if ok {
					dhcp := "off"
					if sub.EnableDHCP {
						dhcp = "on"
					}
					subLine := fmt.Sprintf("      %s  CIDR: %s  GW: %s  IPv%d  DHCP: %s",
						sub.Name,
						sub.CIDR,
						sub.GatewayIP,
						sub.IPVersion,
						dhcp,
					)
					lines = append(lines, line{text: subStyle.Render(subLine)})
				} else {
					lines = append(lines, line{text: subStyle.Render("      " + subID[:8] + "...")})
				}
			}
		}
	}

	// Viewport
	th := m.tableHeight()
	end := m.scrollOff + th
	if end > len(lines) {
		end = len(lines)
	}
	start := m.scrollOff
	if start > len(lines) {
		start = len(lines)
	}
	for _, l := range lines[start:end] {
		b.WriteString(l.text + "\n")
	}

	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "↑↓ navigate • enter expand/collapse • R refresh • 1-5/←→ switch tab • ? help"
}

func (m Model) fetchNetworks() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		nets, err := network.ListNetworks(context.Background(), client)
		if err != nil {
			return networksErrMsg{err: err}
		}
		subs, err := network.ListSubnets(context.Background(), client)
		if err != nil {
			return networksErrMsg{err: err}
		}
		subMap := make(map[string]network.Subnet, len(subs))
		for _, s := range subs {
			subMap[s.ID] = s
		}
		return networksLoadedMsg{networks: nets, subnets: subMap}
	}
}
