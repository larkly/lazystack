package subnetcreate

import (
	"context"
	"fmt"
	"strconv"
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

const (
	fieldName       = 0
	fieldIPVersion  = 1
	fieldSubnetPool = 2
	fieldCIDR       = 3
	fieldGateway    = 4
	fieldDHCP       = 5
	fieldPrefixLen  = 6
	fieldIPv6Cfg    = 7
	fieldIPv6RA     = 8
	fieldSubmit     = 9
	fieldCancel     = 10
	numFields       = 11
)

var (
	ipVersions = []string{"IPv4", "IPv6"}
	dhcpOpts   = []string{"Enabled", "Disabled"}
	ipv6Modes  = []string{"SLAAC", "DHCPv6 stateful", "DHCPv6 stateless", "None"}
)

type subnetCreatedMsg struct{}
type subnetCreateErrMsg struct{ err error }
type subnetPoolsLoadedMsg struct{ pools []network.SubnetPool }
type subnetPoolsFetchErrMsg struct{ err error }

// Model is the subnet create modal.
type Model struct {
	Active         bool
	client         *gophercloud.ServiceClient
	networkID      string
	networkName    string
	nameInput      textinput.Model
	cidrInput      textinput.Model
	gatewayInput   textinput.Model
	prefixLenInput textinput.Model
	ipVersion      int // 0=IPv4, 1=IPv6
	dhcp           int // 0=Enabled, 1=Disabled
	ipv6CfgMode    int // index into ipv6Modes
	ipv6RAMode     int // index into ipv6Modes
	allSubnetPools []network.SubnetPool
	subnetPools    []network.SubnetPool // filtered by IP version
	subnetPool     int                  // 0=None, 1..N=pool index
	loading        bool
	focusField     int
	submitting     bool
	spinner        spinner.Model
	err            string
	width          int
	height         int
}

// New creates a subnet create modal for the given network.
func New(client *gophercloud.ServiceClient, networkID, networkName string) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "subnet name"
	ni.CharLimit = 255
	ni.SetWidth(30)
	ni.Focus()

	ci := textinput.New()
	ci.Prompt = ""
	ci.Placeholder = "e.g. 10.0.0.0/24"
	ci.CharLimit = 43
	ci.SetWidth(25)

	gi := textinput.New()
	gi.Prompt = ""
	gi.Placeholder = "auto (or e.g. 10.0.0.1)"
	gi.CharLimit = 39
	gi.SetWidth(25)

	pi := textinput.New()
	pi.Prompt = ""
	pi.Placeholder = "e.g. 64 (for subnet pool)"
	pi.CharLimit = 3
	pi.SetWidth(25)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:       true,
		client:       client,
		networkID:    networkID,
		networkName:  networkName,
		nameInput:    ni,
		cidrInput:    ci,
		gatewayInput: gi,
		prefixLenInput: pi,
		ipv6CfgMode:  0, // SLAAC by default
		ipv6RAMode:   0, // SLAAC by default
		loading:      true,
		spinner:      s,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchSubnetPools())
}

func (m Model) fetchSubnetPools() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		pools, err := network.ListSubnetPools(context.Background(), client)
		if err != nil {
			return subnetPoolsFetchErrMsg{err: err}
		}
		return subnetPoolsLoadedMsg{pools: pools}
	}
}

func (m *Model) filterSubnetPools() {
	ipVer := 4
	if m.ipVersion == 1 {
		ipVer = 6
	}
	m.subnetPools = nil
	for _, p := range m.allSubnetPools {
		if p.IPVersion == ipVer {
			m.subnetPools = append(m.subnetPools, p)
		}
	}
	if m.subnetPool > len(m.subnetPools) {
		m.subnetPool = 0
	}
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case subnetCreatedMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Created subnet in", Name: m.networkName}
		}
	case subnetCreateErrMsg:
		m.submitting = false
		m.err = msg.err.Error()
		return m, nil
	case subnetPoolsLoadedMsg:
		m.loading = false
		m.allSubnetPools = msg.pools
		m.filterSubnetPools()
		return m, nil
	case subnetPoolsFetchErrMsg:
		m.loading = false
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
		if m.submitting {
			return m, nil
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) isTextInput() bool {
	return m.focusField == fieldName || m.focusField == fieldCIDR || m.focusField == fieldGateway || m.focusField == fieldPrefixLen
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Route to text input first — only intercept navigation keys
	if m.isTextInput() {
		switch {
		case key.Matches(msg, shared.Keys.Back):
			m.Active = false
			return m, nil
		case key.Matches(msg, shared.Keys.Tab):
			m.focusField = (m.focusField + 1) % numFields
			m.updateFocus()
			return m, nil
		case key.Matches(msg, shared.Keys.ShiftTab):
			m.focusField = (m.focusField - 1 + numFields) % numFields
			m.updateFocus()
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			m.focusField++
			m.updateFocus()
			return m, nil
		case msg.String() == "ctrl+s":
			return m.submit()
		default:
			switch m.focusField {
			case fieldName:
				var cmd tea.Cmd
				m.nameInput, cmd = m.nameInput.Update(msg)
				return m, cmd
			case fieldCIDR:
				var cmd tea.Cmd
				m.cidrInput, cmd = m.cidrInput.Update(msg)
				return m, cmd
			case fieldGateway:
				var cmd tea.Cmd
				m.gatewayInput, cmd = m.gatewayInput.Update(msg)
				return m, cmd
			case fieldPrefixLen:
				var cmd tea.Cmd
				m.prefixLenInput, cmd = m.prefixLenInput.Update(msg)
				return m, cmd
			}
		}
	}

	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.Active = false
		return m, nil

	case key.Matches(msg, shared.Keys.Tab), key.Matches(msg, shared.Keys.Down):
		m.focusField = (m.focusField + 1) % numFields
		m.updateFocus()
		return m, nil

	case key.Matches(msg, shared.Keys.ShiftTab), key.Matches(msg, shared.Keys.Up):
		m.focusField = (m.focusField - 1 + numFields) % numFields
		m.updateFocus()
		return m, nil

	case key.Matches(msg, shared.Keys.Right):
		switch m.focusField {
		case fieldIPVersion:
			m.ipVersion = (m.ipVersion + 1) % len(ipVersions)
			m.filterSubnetPools()
			return m, nil
		case fieldSubnetPool:
			count := len(m.subnetPools) + 1
			m.subnetPool = (m.subnetPool + 1) % count
			return m, nil
		case fieldDHCP:
			m.dhcp = (m.dhcp + 1) % len(dhcpOpts)
			return m, nil
		case fieldIPv6Cfg:
			m.ipv6CfgMode = (m.ipv6CfgMode + 1) % len(ipv6Modes)
			return m, nil
		case fieldIPv6RA:
			m.ipv6RAMode = (m.ipv6RAMode + 1) % len(ipv6Modes)
			return m, nil
		case fieldSubmit:
			m.focusField = fieldCancel
			return m, nil
		case fieldCancel:
			m.focusField = fieldSubmit
			return m, nil
		}

	case key.Matches(msg, shared.Keys.Left):
		switch m.focusField {
		case fieldIPVersion:
			m.ipVersion = (m.ipVersion - 1 + len(ipVersions)) % len(ipVersions)
			m.filterSubnetPools()
			return m, nil
		case fieldSubnetPool:
			count := len(m.subnetPools) + 1
			m.subnetPool = (m.subnetPool - 1 + count) % count
			return m, nil
		case fieldDHCP:
			m.dhcp = (m.dhcp - 1 + len(dhcpOpts)) % len(dhcpOpts)
			return m, nil
		case fieldIPv6Cfg:
			m.ipv6CfgMode = (m.ipv6CfgMode - 1 + len(ipv6Modes)) % len(ipv6Modes)
			return m, nil
		case fieldIPv6RA:
			m.ipv6RAMode = (m.ipv6RAMode - 1 + len(ipv6Modes)) % len(ipv6Modes)
			return m, nil
		case fieldSubmit:
			m.focusField = fieldCancel
			return m, nil
		case fieldCancel:
			m.focusField = fieldSubmit
			return m, nil
		}

	case key.Matches(msg, shared.Keys.Enter):
		switch m.focusField {
		case fieldName, fieldCIDR, fieldIPVersion, fieldGateway, fieldDHCP, fieldPrefixLen, fieldIPv6Cfg, fieldIPv6RA:
			m.focusField++
			m.updateFocus()
			return m, nil
		case fieldSubmit:
			return m.submit()
		case fieldCancel:
			m.Active = false
			return m, nil
		}
	}

	if msg.String() == "ctrl+s" {
		return m.submit()
	}

	switch m.focusField {
	case fieldName:
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	case fieldCIDR:
		var cmd tea.Cmd
		m.cidrInput, cmd = m.cidrInput.Update(msg)
		return m, cmd
	case fieldGateway:
		var cmd tea.Cmd
		m.gatewayInput, cmd = m.gatewayInput.Update(msg)
		return m, cmd
	case fieldPrefixLen:
		var cmd tea.Cmd
		m.prefixLenInput, cmd = m.prefixLenInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *Model) updateFocus() {
	if m.focusField == fieldName {
		m.nameInput.Focus()
	} else {
		m.nameInput.Blur()
	}
	if m.focusField == fieldCIDR {
		m.cidrInput.Focus()
	} else {
		m.cidrInput.Blur()
	}
	if m.focusField == fieldGateway {
		m.gatewayInput.Focus()
	} else {
		m.gatewayInput.Blur()
	}
	if m.focusField == fieldPrefixLen {
		m.prefixLenInput.Focus()
	} else {
		m.prefixLenInput.Blur()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	cidr := strings.TrimSpace(m.cidrInput.Value())
	poolSelected := m.subnetPool > 0 && m.subnetPool <= len(m.subnetPools)
	if cidr == "" && !poolSelected {
		m.err = "CIDR is required (or select a subnet pool)"
		return m, nil
	}

	ipVer := 4
	if m.ipVersion == 1 {
		ipVer = 6
	}

	prefixLen := 0
	if ipVer == 6 && poolSelected {
		prefixRaw := strings.TrimSpace(m.prefixLenInput.Value())
		if prefixRaw != "" {
			v, err := strconv.Atoi(prefixRaw)
			if err != nil || v < 1 || v > 128 {
				m.err = "Network mask must be a number between 1 and 128"
				return m, nil
			}
			prefixLen = v
		}
	}

	opts := network.SubnetCreateOpts{
		NetworkID:  m.networkID,
		Name:       strings.TrimSpace(m.nameInput.Value()),
		CIDR:       cidr,
		IPVersion:  ipVer,
		GatewayIP:  strings.TrimSpace(m.gatewayInput.Value()),
		EnableDHCP: m.dhcp == 0,
		PrefixLen:  prefixLen,
	}
	if poolSelected {
		opts.SubnetPoolID = m.subnetPools[m.subnetPool-1].ID
	}
	if ipVer == 6 {
		opts.IPv6AddressMode = ipv6ModeValue(m.ipv6CfgMode)
		opts.IPv6RAMode = ipv6ModeValue(m.ipv6RAMode)
	}

	m.submitting = true
	m.err = ""
	client := m.client

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		shared.Debugf("[subnetcreate] creating subnet %q in network %s (cidr=%s)", opts.Name, m.networkID, cidr)
		_, err := network.CreateSubnet(context.Background(), client, opts)
		if err != nil {
			shared.Debugf("[subnetcreate] error creating subnet %q: %v", opts.Name, err)
			return subnetCreateErrMsg{err: err}
		}
		shared.Debugf("[subnetcreate] created subnet %q in network %s", opts.Name, m.networkName)
		return subnetCreatedMsg{}
	})
}

// View renders the modal.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Create Subnet in " + m.networkName)

	var body strings.Builder

	if m.submitting {
		body.WriteString(m.spinner.View() + " Creating...")
		content := title + "\n\n" + body.String()
		return m.renderModal(content)
	}

	if m.err != "" {
		body.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("⚠ "+m.err) + "\n\n")
	}

	type field struct {
		label   string
		value   string
		focused bool
	}
	fields := []field{
		{"Name", m.nameInput.View(), m.focusField == fieldName},
		{"IP Version", cycleDisplay(ipVersions, m.ipVersion), m.focusField == fieldIPVersion},
		{"Subnet Pool", m.subnetPoolDisplay(), m.focusField == fieldSubnetPool},
		{"CIDR", m.cidrInput.View(), m.focusField == fieldCIDR},
		{"Gateway IP", m.gatewayInput.View(), m.focusField == fieldGateway},
		{"DHCP", cycleDisplay(dhcpOpts, m.dhcp), m.focusField == fieldDHCP},
	}
	if m.ipVersion == 1 {
		fields = append(fields,
			field{"Network Mask", m.prefixLenInput.View(), m.focusField == fieldPrefixLen},
			field{"Configuration Mode", cycleDisplay(ipv6Modes, m.ipv6CfgMode), m.focusField == fieldIPv6Cfg},
			field{"RA Mode", cycleDisplay(ipv6Modes, m.ipv6RAMode), m.focusField == fieldIPv6RA},
		)
	}

	for _, f := range fields {
		cursor := "  "
		if f.focused {
			cursor = "▸ "
		}
		label := lipgloss.NewStyle().Width(14).Foreground(shared.ColorSecondary).Render(f.label)
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if f.focused {
			style = style.Foreground(shared.ColorHighlight)
		}
		body.WriteString(fmt.Sprintf("%s%s %s\n", cursor, label, style.Render(f.value)))
	}

	body.WriteString("\n")
	submitStyle := lipgloss.NewStyle().Padding(0, 2).Background(shared.ColorMuted).Foreground(shared.ColorFg)
	cancelStyle := lipgloss.NewStyle().Padding(0, 2).Background(shared.ColorMuted).Foreground(shared.ColorFg)
	if m.focusField == fieldSubmit {
		submitStyle = submitStyle.Background(shared.ColorSuccess).Foreground(shared.ColorBg).Bold(true)
	}
	if m.focusField == fieldCancel {
		cancelStyle = cancelStyle.Background(shared.ColorError).Foreground(shared.ColorBg).Bold(true)
	}
	body.WriteString("  " + submitStyle.Render("[ctrl+s] Submit") + "  " + cancelStyle.Render("[esc] Cancel") + "\n")
	body.WriteString("\n")
	body.WriteString(shared.StyleHelp.Render("  tab/↑↓ fields • ←→ cycle • ctrl+s submit • esc cancel"))

	content := title + "\n\n" + body.String()
	return m.renderModal(content)
}

func (m Model) subnetPoolDisplay() string {
	if m.loading {
		return m.spinner.View() + " Loading..."
	}
	if len(m.subnetPools) == 0 {
		return lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("None available")
	}
	label := "None"
	if m.subnetPool > 0 && m.subnetPool <= len(m.subnetPools) {
		p := m.subnetPools[m.subnetPool-1]
		label = p.Name
		if len(p.Prefixes) > 0 {
			prefixStr := strings.Join(p.Prefixes, ", ")
			if len(prefixStr) > 25 {
				prefixStr = prefixStr[:22] + "..."
			}
			label += " [" + prefixStr + "]"
		}
	}
	return fmt.Sprintf("◀ %s ▶", label)
}

func cycleDisplay(options []string, selected int) string {
	var parts []string
	for i, opt := range options {
		if i == selected {
			parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(shared.ColorHighlight).Render("● "+opt))
		} else {
			parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("○ "+opt))
		}
	}
	return strings.Join(parts, "  ")
}

func ipv6ModeValue(idx int) string {
	switch idx {
	case 0:
		return "slaac"
	case 1:
		return "dhcpv6-stateful"
	case 2:
		return "dhcpv6-stateless"
	default:
		return ""
	}
}

func (m Model) renderModal(content string) string {
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
