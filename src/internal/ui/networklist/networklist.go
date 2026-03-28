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
	ports    map[string][]network.Port // NetworkID → Ports
}
type networksErrMsg struct{ err error }
type tickMsg struct{}

// Model is the network browser view.
type Model struct {
	client          *gophercloud.ServiceClient
	networks        []network.Network
	subnets         map[string]network.Subnet
	ports           map[string][]network.Port // NetworkID → Ports
	cursor          int
	expanded        map[string]bool // network ID → expanded
	inSubnets       bool
	subnetCursor    int
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
		expanded:        make(map[string]bool),
		subnets:         make(map[string]network.Subnet),
		refreshInterval: refreshInterval,
	}
}

// Init starts the initial fetch and auto-refresh ticker.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchNetworks(), m.tickCmd())
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
		m.ports = msg.ports
		m.err = ""
		if m.cursor >= len(m.networks) && len(m.networks) > 0 {
			m.cursor = len(m.networks) - 1
			m.inSubnets = false
		}
		return m, nil
	case networksErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.fetchNetworks(), m.tickCmd())
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

func (m Model) isExpanded(idx int) bool {
	if idx < 0 || idx >= len(m.networks) {
		return false
	}
	return m.expanded[m.networks[idx].ID]
}

func (m *Model) toggleExpanded(idx int) {
	if idx < 0 || idx >= len(m.networks) {
		return
	}
	id := m.networks[idx].ID
	m.expanded[id] = !m.expanded[id]
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.inSubnets {
		return m.handleSubnetKey(msg)
	}

	switch {
	case key.Matches(msg, shared.Keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.ensureVisible()
		}
	case key.Matches(msg, shared.Keys.Down):
		if m.isExpanded(m.cursor) && len(m.networks[m.cursor].SubnetIDs) > 0 {
			m.inSubnets = true
			m.subnetCursor = 0
		} else if m.cursor < len(m.networks)-1 {
			m.cursor++
			m.ensureVisible()
		}
	case key.Matches(msg, shared.Keys.Enter):
		m.toggleExpanded(m.cursor)
		if !m.isExpanded(m.cursor) {
			m.inSubnets = false
		}
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

func (m Model) handleSubnetKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	subnetIDs := m.networks[m.cursor].SubnetIDs
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.inSubnets = false
		return m, nil
	case key.Matches(msg, shared.Keys.Up):
		if m.subnetCursor > 0 {
			m.subnetCursor--
		} else {
			m.inSubnets = false
		}
	case key.Matches(msg, shared.Keys.Down):
		if m.subnetCursor < len(subnetIDs)-1 {
			m.subnetCursor++
		} else {
			m.inSubnets = false
			if m.cursor < len(m.networks)-1 {
				m.cursor++
				m.ensureVisible()
			}
		}
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
	statusW := 12
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
			statusStyle.Width(statusW).Render(shared.StatusIcon(net.Status) + net.Status),
			subnetsW, len(net.SubnetIDs),
			sharedW, sharedStr,
		)
		if isCurrent {
			lines = append(lines, line{text: style.Render(row)})
		} else {
			lines = append(lines, line{text: style.Render(row)})
		}

		// Show expanded subnets
		if m.isExpanded(i) {
			for j, subID := range net.SubnetIDs {
				sub, ok := m.subnets[subID]
				isSubSel := m.inSubnets && i == m.cursor && j == m.subnetCursor

				subStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
				prefix := "      "
				if isSubSel {
					subStyle = subStyle.Foreground(shared.ColorHighlight).Bold(true)
					prefix = "    ▸ "
				}

				if ok {
					dhcp := "off"
					if sub.EnableDHCP {
						dhcp = "on"
					}
					subLine := fmt.Sprintf("%s%s  CIDR: %s  GW: %s  IPv%d  DHCP: %s",
						prefix,
						sub.Name,
						sub.CIDR,
						sub.GatewayIP,
						sub.IPVersion,
						dhcp,
					)
					lines = append(lines, line{text: subStyle.Render(subLine)})
				} else {
					lines = append(lines, line{text: subStyle.Render(prefix + subID[:8] + "...")})
				}
			}
			// Show ports for this network
			if netPorts, ok := m.ports[net.ID]; ok && len(netPorts) > 0 {
				portStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
				for _, p := range netPorts {
					var ips []string
					for _, ip := range p.FixedIPs {
						ips = append(ips, ip.IPAddress)
					}
					ipStr := strings.Join(ips, ", ")
					name := p.Name
					if name == "" {
						name = p.ID[:8]
					}
					portLine := fmt.Sprintf("      port: %s  MAC: %s  IPs: %s  %s",
						name, p.MACAddress, ipStr, p.DeviceOwner)
					lines = append(lines, line{text: portStyle.Render(portLine)})
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

// InSubnets returns true when navigating subnets within an expanded network.
func (m Model) InSubnets() bool {
	return m.inSubnets
}

// IsExpanded returns true if the network at the cursor is expanded.
func (m Model) IsExpanded() bool {
	return m.isExpanded(m.cursor)
}

// SelectedNetworkID returns the ID of the network at the cursor.
func (m Model) SelectedNetworkID() string {
	if m.cursor >= 0 && m.cursor < len(m.networks) {
		return m.networks[m.cursor].ID
	}
	return ""
}

// SelectedNetworkName returns the name of the network at the cursor.
func (m Model) SelectedNetworkName() string {
	if m.cursor >= 0 && m.cursor < len(m.networks) {
		return m.networks[m.cursor].Name
	}
	return ""
}

// SelectedSubnetID returns the ID of the selected subnet (when in subnet navigation).
func (m Model) SelectedSubnetID() string {
	if !m.inSubnets || m.cursor < 0 || m.cursor >= len(m.networks) {
		return ""
	}
	ids := m.networks[m.cursor].SubnetIDs
	if m.subnetCursor < 0 || m.subnetCursor >= len(ids) {
		return ""
	}
	return ids[m.subnetCursor]
}

// SelectedSubnetName returns the name of the selected subnet.
func (m Model) SelectedSubnetName() string {
	id := m.SelectedSubnetID()
	if id == "" {
		return ""
	}
	if sub, ok := m.subnets[id]; ok {
		return sub.Name
	}
	return id[:8] + "..."
}

// Hints returns key hints.
func (m Model) Hints() string {
	if m.inSubnets {
		return "↑↓ navigate subnets • ^n create subnet • ^d delete subnet • esc back • R refresh • ? help"
	}
	if m.isExpanded(m.cursor) {
		return "↑↓ navigate • enter collapse • ^n create subnet • ^d delete network • R refresh • 1-5/←→ switch tab • ? help"
	}
	return "↑↓ navigate • enter expand • ^n create network • ^d delete network • R refresh • 1-5/←→ switch tab • ? help"
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
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
		// Fetch ports for all networks
		portMap := make(map[string][]network.Port)
		for _, n := range nets {
			ps, err := network.ListPorts(context.Background(), client, n.ID)
			if err == nil && len(ps) > 0 {
				portMap[n.ID] = ps
			}
		}
		return networksLoadedMsg{networks: nets, subnets: subMap, ports: portMap}
	}
}
