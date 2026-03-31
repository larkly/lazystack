package subnetpicker

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type phase int

const (
	phaseList    phase = iota // selecting a subnet
	phaseConfirm              // editing IP and confirming
)

const (
	confirmIP     = 0
	confirmSubmit = 1
	confirmCancel = 2
	confirmFields = 3
)

type subnetsLoadedMsg struct{ subnets []network.Subnet }
type fetchErrMsg struct{ err error }
type interfaceAddedMsg struct{ routerName string }
type interfaceAddErrMsg struct{ err error }

// Model is the subnet picker modal for adding router interfaces.
type Model struct {
	Active         bool
	client         *gophercloud.ServiceClient
	routerID       string
	routerName     string
	subnets        []network.Subnet
	cursor         int
	scrollOff      int
	loading        bool
	submitting     bool
	spinner        spinner.Model
	err            string
	width          int
	height         int
	phase          phase
	selectedSubnet network.Subnet
	ipInput        textinput.Model
	focusField     int
}

// New creates a subnet picker for the given router.
func New(client *gophercloud.ServiceClient, routerID, routerName string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		Active:     true,
		client:     client,
		routerID:   routerID,
		routerName: routerName,
		loading:    true,
		spinner:    s,
	}
}

// Init fetches all subnets.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchSubnets())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case subnetsLoadedMsg:
		m.loading = false
		m.subnets = msg.subnets
		return m, nil

	case fetchErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case interfaceAddedMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Added interface to", Name: msg.routerName}
		}

	case interfaceAddErrMsg:
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
		if m.phase == phaseConfirm {
			return m.handleConfirmKey(msg)
		}
		return m.handleListKey(msg)
	}
	return m, nil
}

func (m Model) handleListKey(msg tea.KeyMsg) (Model, tea.Cmd) {
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
		if m.cursor < len(m.subnets)-1 {
			m.cursor++
			m.ensureVisible()
		}
	case key.Matches(msg, shared.Keys.Enter):
		if len(m.subnets) > 0 && m.cursor < len(m.subnets) {
			m.selectedSubnet = m.subnets[m.cursor]
			m.phase = phaseConfirm
			m.focusField = confirmIP
			m.err = ""

			ip := textinput.New()
			ip.Prompt = ""
			ip.Placeholder = "e.g. 10.0.0.1"
			ip.CharLimit = 45 // enough for IPv6
			ip.SetWidth(30)
			ip.Focus()

			defaultIP := m.selectedSubnet.GatewayIP
			if defaultIP == "" {
				defaultIP = firstUsableIP(m.selectedSubnet.CIDR)
			}
			ip.SetValue(defaultIP)
			m.ipInput = ip
			return m, nil
		}
	}
	return m, nil
}

func (m Model) handleConfirmKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.focusField == confirmIP {
		switch {
		case key.Matches(msg, shared.Keys.Back):
			m.phase = phaseList
			m.err = ""
			return m, nil
		case key.Matches(msg, shared.Keys.Tab):
			m.focusField = confirmSubmit
			m.ipInput.Blur()
			return m, nil
		case key.Matches(msg, shared.Keys.ShiftTab):
			m.focusField = confirmCancel
			m.ipInput.Blur()
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			m.focusField = confirmSubmit
			m.ipInput.Blur()
			return m, nil
		case msg.String() == "ctrl+s":
			return m.submitInterface()
		default:
			var cmd tea.Cmd
			m.ipInput, cmd = m.ipInput.Update(msg)
			return m, cmd
		}
	}

	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.phase = phaseList
		m.err = ""
		return m, nil
	case key.Matches(msg, shared.Keys.Tab), key.Matches(msg, shared.Keys.Down):
		m.focusField = (m.focusField + 1) % confirmFields
		m.updateConfirmFocus()
		return m, nil
	case key.Matches(msg, shared.Keys.ShiftTab), key.Matches(msg, shared.Keys.Up):
		m.focusField = (m.focusField - 1 + confirmFields) % confirmFields
		m.updateConfirmFocus()
		return m, nil
	case key.Matches(msg, shared.Keys.Left), key.Matches(msg, shared.Keys.Right):
		if m.focusField == confirmSubmit {
			m.focusField = confirmCancel
		} else if m.focusField == confirmCancel {
			m.focusField = confirmSubmit
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		switch m.focusField {
		case confirmSubmit:
			return m.submitInterface()
		case confirmCancel:
			m.phase = phaseList
			m.err = ""
			return m, nil
		}
	case msg.String() == "ctrl+s":
		return m.submitInterface()
	}

	return m, nil
}

func (m *Model) updateConfirmFocus() {
	if m.focusField == confirmIP {
		m.ipInput.Focus()
	} else {
		m.ipInput.Blur()
	}
}

func (m Model) submitInterface() (Model, tea.Cmd) {
	ipStr := strings.TrimSpace(m.ipInput.Value())
	if ipStr == "" {
		m.err = "IP address is required"
		return m, nil
	}
	if net.ParseIP(ipStr) == nil {
		m.err = "Invalid IP address"
		return m, nil
	}

	_, ipNet, err := net.ParseCIDR(m.selectedSubnet.CIDR)
	if err == nil && !ipNet.Contains(net.ParseIP(ipStr)) {
		m.err = fmt.Sprintf("IP %s is not within %s", ipStr, m.selectedSubnet.CIDR)
		return m, nil
	}

	m.submitting = true
	m.err = ""

	client := m.client
	routerID := m.routerID
	routerName := m.routerName
	sub := m.selectedSubnet

	// Always use the PortID path with explicit fixed_ips to prevent Neutron
	// from auto-assigning addresses from other subnets on the same network.
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx := context.Background()

		shared.Debugf("[subnetpicker] creating port (subnet %s, ip %s) for router %s (%s)", sub.ID, ipStr, routerID, routerName)
		port, err := network.CreatePort(ctx, client, sub.NetworkID, sub.ID, ipStr)
		if err != nil {
			shared.Debugf("[subnetpicker] error creating port for router %s: %v", routerID, err)
			return interfaceAddErrMsg{err: err}
		}

		shared.Debugf("[subnetpicker] adding port %s to router %s (%s)", port.ID, routerID, routerName)
		err = network.AddRouterInterfaceByPort(ctx, client, routerID, port.ID)
		if err != nil {
			shared.Debugf("[subnetpicker] error adding port to router %s, cleaning up port %s: %v", routerID, port.ID, err)
			_ = network.DeletePort(ctx, client, port.ID)
			return interfaceAddErrMsg{err: err}
		}

		shared.Debugf("[subnetpicker] added interface (port %s, ip %s) to router %s", port.ID, ipStr, routerName)
		return interfaceAddedMsg{routerName: routerName}
	})
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
	h := m.height - 12 // modal chrome
	if h < 3 {
		h = 3
	}
	return h
}

// View renders the subnet picker modal.
func (m Model) View() string {
	if m.phase == phaseConfirm {
		return m.viewConfirm()
	}
	return m.viewList()
}

func (m Model) viewList() string {
	title := shared.StyleModalTitle.Render("Add Interface to " + m.routerName)

	var body string
	if m.loading {
		body = m.spinner.View() + " Loading subnets..."
	} else if m.submitting {
		body = m.spinner.View() + " Adding interface..."
	} else if m.err != "" {
		body = lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.err)
		body += "\n\n" + shared.StyleHelp.Render("esc to close")
	} else if len(m.subnets) == 0 {
		body = lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("No subnets available")
		body += "\n\n" + shared.StyleHelp.Render("esc to close")
	} else {
		var lines []string
		th := m.listHeight()
		end := m.scrollOff + th
		if end > len(m.subnets) {
			end = len(m.subnets)
		}

		for i := m.scrollOff; i < end; i++ {
			cursor := "  "
			if i == m.cursor {
				cursor = "▸ "
			}

			sub := m.subnets[i]
			style := lipgloss.NewStyle().Foreground(shared.ColorFg)
			if i == m.cursor {
				style = style.Foreground(shared.ColorHighlight).Bold(true)
			}
			name := sub.Name
			if name == "" {
				name = sub.ID[:8]
			}
			cidr := lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(" " + sub.CIDR)
			lines = append(lines, fmt.Sprintf("%s%s%s", cursor, style.Render(name), cidr))
		}
		body = strings.Join(lines, "\n")
		body += "\n\n" + shared.StyleHelp.Render("↑↓ navigate • enter select • esc cancel")
	}

	content := title + "\n\n" + body
	modalWidth := 50
	if m.width > 0 && m.width < 60 {
		modalWidth = m.width - 6
	}
	box := shared.StyleModal.Width(modalWidth).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) viewConfirm() string {
	title := shared.StyleModalTitle.Render("Add Interface to " + m.routerName)

	var body strings.Builder

	if m.submitting {
		body.WriteString(m.spinner.View() + " Adding interface...")
		content := title + "\n\n" + body.String()
		return m.renderModal(content)
	}

	if m.err != "" {
		body.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("⚠ "+m.err) + "\n\n")
	}

	subName := m.selectedSubnet.Name
	if subName == "" {
		subName = m.selectedSubnet.ID[:8]
	}
	subLine := lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true).Render(subName)
	subLine += lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(" " + m.selectedSubnet.CIDR)
	body.WriteString("  " + subLine + "\n\n")

	// IP address field
	cursor := "  "
	if m.focusField == confirmIP {
		cursor = "▸ "
	}
	label := lipgloss.NewStyle().Width(14).Foreground(shared.ColorSecondary).Render("IP Address")
	ipStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
	if m.focusField == confirmIP {
		ipStyle = ipStyle.Foreground(shared.ColorHighlight)
	}
	body.WriteString(fmt.Sprintf("%s%s %s\n", cursor, label, ipStyle.Render(m.ipInput.View())))

	// Show gateway hint
	if m.selectedSubnet.GatewayIP != "" {
		gwLabel := lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(
			fmt.Sprintf("                 gateway: %s", m.selectedSubnet.GatewayIP))
		body.WriteString(gwLabel + "\n")
	}

	body.WriteString("\n")

	submitStyle := lipgloss.NewStyle().Padding(0, 2).Background(shared.ColorMuted).Foreground(shared.ColorFg)
	cancelStyle := lipgloss.NewStyle().Padding(0, 2).Background(shared.ColorMuted).Foreground(shared.ColorFg)
	if m.focusField == confirmSubmit {
		submitStyle = submitStyle.Background(shared.ColorSuccess).Foreground(shared.ColorBg).Bold(true)
	}
	if m.focusField == confirmCancel {
		cancelStyle = cancelStyle.Background(shared.ColorError).Foreground(shared.ColorBg).Bold(true)
	}
	body.WriteString("  " + submitStyle.Render("[ctrl+s] Submit") + "  " + cancelStyle.Render("[esc] Cancel") + "\n")
	body.WriteString("\n")
	body.WriteString(shared.StyleHelp.Render("  tab fields • ctrl+s submit • esc back"))

	content := title + "\n\n" + body.String()
	return m.renderModal(content)
}

func (m Model) renderModal(content string) string {
	modalWidth := 55
	if m.width > 0 && m.width < 65 {
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

func (m Model) fetchSubnets() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		subnets, err := network.ListSubnets(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return subnetsLoadedMsg{subnets: subnets}
	}
}

// firstUsableIP returns the first host address (.1 or ::1) from a CIDR.
func firstUsableIP(cidr string) string {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return ""
	}
	ip := make(net.IP, len(ipNet.IP))
	copy(ip, ipNet.IP)
	ip[len(ip)-1] = 1
	return ip.String()
}
