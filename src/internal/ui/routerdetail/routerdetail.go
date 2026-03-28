package routerdetail

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

type routerLoadedMsg struct {
	router       *network.Router
	interfaces   []network.RouterInterface
	networkNames map[string]string
	subnetNames  map[string]string
}

type routerErrMsg struct {
	err error
}

type tickMsg struct{}

// Model is the router detail view.
type Model struct {
	client          *gophercloud.ServiceClient
	routerID        string
	router          *network.Router
	interfaces      []network.RouterInterface
	networkNames    map[string]string
	subnetNames     map[string]string
	scroll          int
	interfaceCursor int
	loading         bool
	spinner         spinner.Model
	err             string
	width           int
	height          int
	refreshInterval time.Duration
}

// New creates a router detail model.
func New(client *gophercloud.ServiceClient, routerID string, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:          client,
		routerID:        routerID,
		loading:         true,
		spinner:         s,
		networkNames:    make(map[string]string),
		subnetNames:     make(map[string]string),
		refreshInterval: refreshInterval,
	}
}

// Init fetches the router details.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchDetail(), m.tickCmd())
}

// RouterID returns the current router ID.
func (m Model) RouterID() string {
	return m.routerID
}

// RouterName returns the current router name.
func (m Model) RouterName() string {
	if m.router != nil {
		if m.router.Name != "" {
			return m.router.Name
		}
		return m.routerID
	}
	return m.routerID
}

// SelectedInterfaceSubnetID returns the subnet ID of the currently selected interface.
func (m Model) SelectedInterfaceSubnetID() string {
	if len(m.interfaces) == 0 {
		return ""
	}
	if m.interfaceCursor < 0 || m.interfaceCursor >= len(m.interfaces) {
		return ""
	}
	return m.interfaces[m.interfaceCursor].SubnetID
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case routerLoadedMsg:
		m.loading = false
		m.router = msg.router
		m.interfaces = msg.interfaces
		m.networkNames = msg.networkNames
		m.subnetNames = msg.subnetNames
		m.err = ""
		if m.interfaceCursor >= len(m.interfaces) {
			m.interfaceCursor = max(0, len(m.interfaces)-1)
		}
		return m, nil

	case routerErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.fetchDetail(), m.tickCmd())

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
				return shared.ViewChangeMsg{View: "routerlist"}
			}
		case key.Matches(msg, shared.Keys.Up):
			if m.scroll > 0 {
				m.scroll--
			}
			if m.interfaceCursor > 0 {
				m.interfaceCursor--
			}
		case key.Matches(msg, shared.Keys.Down):
			m.scroll++
			if m.interfaceCursor < len(m.interfaces)-1 {
				m.interfaceCursor++
			}
		case key.Matches(msg, shared.Keys.PageDown):
			m.scroll += m.height - 5
		case key.Matches(msg, shared.Keys.PageUp):
			m.scroll -= m.height - 5
			if m.scroll < 0 {
				m.scroll = 0
			}
		}
	}
	return m, nil
}

// View renders the router detail.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Router: " + m.RouterName())
	if m.loading {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if m.router == nil {
		return b.String()
	}

	r := m.router
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(20)

	lines := make([]string, 0, 32)

	// Properties
	adminState := "Down"
	if r.AdminStateUp {
		adminState = "Up"
	}

	props := []struct {
		label string
		value string
	}{
		{"Name", r.Name},
		{"ID", r.ID},
		{"Status", r.Status},
		{"Admin State", adminState},
		{"Description", r.Description},
	}

	for _, p := range props {
		if p.value == "" {
			continue
		}
		label := labelStyle.Render(p.label)
		var value string
		if p.label == "Status" {
			value = statusStyle(p.value).Render(shared.StatusIcon(p.value) + p.value)
		} else if p.label == "Admin State" {
			value = adminStateStyle(p.value).Render(p.value)
		} else {
			value = lipgloss.NewStyle().Foreground(shared.ColorFg).Render(p.value)
		}
		lines = append(lines, fmt.Sprintf("  %s %s", label, value))
	}

	// External Gateway
	gwLabel := labelStyle.Render("External Gateway")
	if r.ExternalGatewayNetworkID != "" {
		netName := m.networkNames[r.ExternalGatewayNetworkID]
		if netName == "" {
			netName = r.ExternalGatewayNetworkID
		}
		gwValue := netName
		if r.ExternalGatewayIP != "" {
			gwValue += "  " + r.ExternalGatewayIP
		}
		lines = append(lines, fmt.Sprintf("  %s %s", gwLabel,
			lipgloss.NewStyle().Foreground(shared.ColorFg).Render(gwValue)))
	} else {
		lines = append(lines, fmt.Sprintf("  %s %s", gwLabel,
			lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("none")))
	}

	// Interfaces section
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  %s",
		lipgloss.NewStyle().Bold(true).Foreground(shared.ColorSecondary).Render("Interfaces")))

	if len(m.interfaces) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("  No interfaces"))
	} else {
		for i, iface := range m.interfaces {
			subnetName := m.subnetNames[iface.SubnetID]
			if subnetName == "" {
				subnetName = iface.SubnetID
			}
			portShort := iface.PortID
			if len(portShort) > 8 {
				portShort = portShort[:8]
			}

			line := fmt.Sprintf("  \u25b8 %s  IP: %s  Port: %s", subnetName, iface.IPAddress, portShort)
			if i == m.interfaceCursor {
				line = lipgloss.NewStyle().Foreground(shared.ColorHighlight).Render(line)
			} else {
				line = lipgloss.NewStyle().Foreground(shared.ColorFg).Render(line)
			}
			lines = append(lines, line)
		}
	}

	// Routes section
	if len(r.Routes) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("  %s",
			lipgloss.NewStyle().Bold(true).Foreground(shared.ColorSecondary).Render("Routes")))
		for _, route := range r.Routes {
			lines = append(lines, lipgloss.NewStyle().Foreground(shared.ColorFg).
				Render(fmt.Sprintf("  %s \u2192 %s", route.DestinationCIDR, route.NextHop)))
		}
	}

	// Scroll
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

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "ACTIVE":
		return lipgloss.NewStyle().Foreground(shared.ColorSuccess)
	case "ERROR":
		return lipgloss.NewStyle().Foreground(shared.ColorError)
	default:
		return lipgloss.NewStyle().Foreground(shared.ColorFg)
	}
}

func adminStateStyle(state string) lipgloss.Style {
	switch state {
	case "Up":
		return lipgloss.NewStyle().Foreground(shared.ColorSuccess)
	case "Down":
		return lipgloss.NewStyle().Foreground(shared.ColorError)
	default:
		return lipgloss.NewStyle().Foreground(shared.ColorFg)
	}
}

func (m Model) fetchDetail() tea.Cmd {
	client := m.client
	id := m.routerID
	return func() tea.Msg {
		ctx := context.Background()

		router, err := network.GetRouter(ctx, client, id)
		if err != nil {
			return routerErrMsg{err: err}
		}

		ifaces, err := network.ListRouterInterfaces(ctx, client, id)
		if err != nil {
			return routerErrMsg{err: err}
		}

		nets, err := network.ListNetworks(ctx, client)
		if err != nil {
			return routerErrMsg{err: err}
		}
		networkNames := make(map[string]string, len(nets))
		for _, n := range nets {
			networkNames[n.ID] = n.Name
		}

		subs, err := network.ListSubnets(ctx, client)
		if err != nil {
			return routerErrMsg{err: err}
		}
		subnetNames := make(map[string]string, len(subs))
		for _, s := range subs {
			subnetNames[s.ID] = s.Name
		}

		return routerLoadedMsg{
			router:       router,
			interfaces:   ifaces,
			networkNames: networkNames,
			subnetNames:  subnetNames,
		}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// ForceRefresh triggers a manual reload of the router detail.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchDetail())
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	return "\u2191\u2193 scroll \u2022 ^a add interface \u2022 ^t remove interface \u2022 ^d delete \u2022 R refresh \u2022 esc back \u2022 ? help"
}
