package subnetedit

import (
	"context"
	"fmt"
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
	fieldGateway    = 1
	fieldDHCP       = 2
	fieldDNS        = 3
	fieldAllocPools = 4
	fieldHostRoutes = 5
	fieldSubmit     = 6
	fieldCancel     = 7
	numFields       = 8
)

var dhcpOpts = []string{"Enabled", "Disabled"}

type subnetUpdatedMsg struct{ name string }
type subnetUpdateErrMsg struct{ err error }

// Model is the subnet edit modal.
type Model struct {
	Active         bool
	client         *gophercloud.ServiceClient
	subnetID       string
	subnet         network.Subnet
	nameInput      textinput.Model
	gatewayInput   textinput.Model
	dnsInput       textinput.Model
	allocPoolInput textinput.Model
	hostRouteInput textinput.Model
	dhcp           int // 0=Enabled, 1=Disabled
	focusField     int
	submitting     bool
	spinner        spinner.Model
	err            string
	width          int
	height         int
}

// New creates a subnet edit modal for the given subnet.
func New(client *gophercloud.ServiceClient, sub network.Subnet) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "subnet name"
	ni.CharLimit = 255
	ni.SetWidth(40)
	ni.SetValue(sub.Name)
	ni.Focus()

	gi := textinput.New()
	gi.Prompt = ""
	gi.Placeholder = "e.g. 10.0.0.1"
	gi.CharLimit = 45
	gi.SetWidth(40)
	gi.SetValue(sub.GatewayIP)

	di := textinput.New()
	di.Prompt = ""
	di.Placeholder = "e.g. 2001:4860:4860::8888, 2001:4860:4860::8844"
	di.CharLimit = 200
	di.SetWidth(40)
	di.SetValue(strings.Join(sub.DNSNameservers, ", "))

	ai := textinput.New()
	ai.Prompt = ""
	ai.Placeholder = "e.g. 10.0.0.2-10.0.0.254"
	ai.CharLimit = 500
	ai.SetWidth(40)
	var poolStrs []string
	for _, p := range sub.AllocationPools {
		poolStrs = append(poolStrs, p.Start+"-"+p.End)
	}
	ai.SetValue(strings.Join(poolStrs, ", "))

	ri := textinput.New()
	ri.Prompt = ""
	ri.Placeholder = "e.g. 172.16.0.0/24>10.0.0.1, 192.168.0.0/16>10.0.0.1"
	ri.CharLimit = 500
	ri.SetWidth(40)
	var routeStrs []string
	for _, r := range sub.HostRoutes {
		routeStrs = append(routeStrs, r.DestinationCIDR+">"+r.NextHop)
	}
	ri.SetValue(strings.Join(routeStrs, ", "))

	dhcp := 0
	if !sub.EnableDHCP {
		dhcp = 1
	}

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:         true,
		client:         client,
		subnetID:       sub.ID,
		subnet:         sub,
		nameInput:      ni,
		gatewayInput:   gi,
		dnsInput:       di,
		allocPoolInput: ai,
		hostRouteInput: ri,
		dhcp:           dhcp,
		spinner:        s,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case subnetUpdatedMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Updated subnet", Name: msg.name}
		}
	case subnetUpdateErrMsg:
		m.submitting = false
		m.err = msg.err.Error()
		return m, nil
	case spinner.TickMsg:
		if m.submitting {
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
	switch m.focusField {
	case fieldName, fieldGateway, fieldDNS, fieldAllocPools, fieldHostRoutes:
		return true
	}
	return false
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
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
			return m.routeToInput(msg)
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
	case key.Matches(msg, shared.Keys.Left), key.Matches(msg, shared.Keys.Right):
		switch m.focusField {
		case fieldDHCP:
			m.dhcp = (m.dhcp + 1) % len(dhcpOpts)
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
		case fieldDHCP:
			m.focusField++
			m.updateFocus()
			return m, nil
		case fieldSubmit:
			return m.submit()
		case fieldCancel:
			m.Active = false
			return m, nil
		}
	case msg.String() == "ctrl+s":
		return m.submit()
	}

	return m.routeToInput(msg)
}

func (m Model) routeToInput(msg tea.KeyMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focusField {
	case fieldName:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case fieldGateway:
		m.gatewayInput, cmd = m.gatewayInput.Update(msg)
	case fieldDNS:
		m.dnsInput, cmd = m.dnsInput.Update(msg)
	case fieldAllocPools:
		m.allocPoolInput, cmd = m.allocPoolInput.Update(msg)
	case fieldHostRoutes:
		m.hostRouteInput, cmd = m.hostRouteInput.Update(msg)
	}
	return m, cmd
}

func (m *Model) updateFocus() {
	m.nameInput.Blur()
	m.gatewayInput.Blur()
	m.dnsInput.Blur()
	m.allocPoolInput.Blur()
	m.hostRouteInput.Blur()
	switch m.focusField {
	case fieldName:
		m.nameInput.Focus()
	case fieldGateway:
		m.gatewayInput.Focus()
	case fieldDNS:
		m.dnsInput.Focus()
	case fieldAllocPools:
		m.allocPoolInput.Focus()
	case fieldHostRoutes:
		m.hostRouteInput.Focus()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	opts := network.SubnetUpdateOpts{}

	name := strings.TrimSpace(m.nameInput.Value())
	if name != m.subnet.Name {
		opts.Name = &name
	}

	gw := strings.TrimSpace(m.gatewayInput.Value())
	if gw != m.subnet.GatewayIP {
		opts.GatewayIP = &gw
	}

	enableDHCP := m.dhcp == 0
	if enableDHCP != m.subnet.EnableDHCP {
		opts.EnableDHCP = &enableDHCP
	}

	dnsRaw := strings.TrimSpace(m.dnsInput.Value())
	var dns []string
	if dnsRaw != "" {
		for _, s := range strings.Split(dnsRaw, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				dns = append(dns, s)
			}
		}
	}
	if !stringSliceEqual(dns, m.subnet.DNSNameservers) {
		opts.DNSNameservers = &dns
	}

	poolsRaw := strings.TrimSpace(m.allocPoolInput.Value())
	pools, err := parsePools(poolsRaw)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if !poolsEqual(pools, m.subnet.AllocationPools) {
		opts.AllocationPools = pools
	}

	routesRaw := strings.TrimSpace(m.hostRouteInput.Value())
	routes, err := parseRoutes(routesRaw)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if !routesEqual(routes, m.subnet.HostRoutes) {
		opts.HostRoutes = &routes
	}

	m.submitting = true
	m.err = ""
	client := m.client
	id := m.subnetID
	displayName := name
	if displayName == "" {
		displayName = id[:8]
	}

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		shared.Debugf("[subnetedit] updating subnet %s", id)
		err := network.UpdateSubnet(context.Background(), client, id, opts)
		if err != nil {
			shared.Debugf("[subnetedit] error updating subnet %s: %v", id, err)
			return subnetUpdateErrMsg{err: err}
		}
		shared.Debugf("[subnetedit] updated subnet %s", id)
		return subnetUpdatedMsg{name: displayName}
	})
}

// View renders the modal.
func (m Model) View() string {
	cidr := lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(" (" + m.subnet.CIDR + ")")
	title := shared.StyleModalTitle.Render("Edit Subnet") + cidr

	var body strings.Builder

	if m.submitting {
		body.WriteString(m.spinner.View() + " Updating...")
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
		{"Gateway IP", m.gatewayInput.View(), m.focusField == fieldGateway},
		{"DHCP", cycleDisplay(dhcpOpts, m.dhcp), m.focusField == fieldDHCP},
		{"DNS Servers", m.dnsInput.View(), m.focusField == fieldDNS},
		{"Alloc Pools", m.allocPoolInput.View(), m.focusField == fieldAllocPools},
		{"Host Routes", m.hostRouteInput.View(), m.focusField == fieldHostRoutes},
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

func (m Model) renderModal(content string) string {
	modalWidth := 86
	if m.width > 0 && m.width < 96 {
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

// parsePools parses "start-end, start-end" format.
func parsePools(raw string) ([]network.AllocationPool, error) {
	if raw == "" {
		return nil, nil
	}
	var pools []network.AllocationPool
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pieces := strings.SplitN(part, "-", 2)
		if len(pieces) != 2 {
			return nil, fmt.Errorf("invalid pool %q (expected start-end)", part)
		}
		pools = append(pools, network.AllocationPool{
			Start: strings.TrimSpace(pieces[0]),
			End:   strings.TrimSpace(pieces[1]),
		})
	}
	return pools, nil
}

// parseRoutes parses "cidr>nexthop, cidr>nexthop" format.
func parseRoutes(raw string) ([]network.HostRoute, error) {
	if raw == "" {
		return nil, nil
	}
	var routes []network.HostRoute
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pieces := strings.SplitN(part, ">", 2)
		if len(pieces) != 2 {
			return nil, fmt.Errorf("invalid route %q (expected cidr>nexthop)", part)
		}
		routes = append(routes, network.HostRoute{
			DestinationCIDR: strings.TrimSpace(pieces[0]),
			NextHop:         strings.TrimSpace(pieces[1]),
		})
	}
	return routes, nil
}

func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func poolsEqual(a, b []network.AllocationPool) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func routesEqual(a, b []network.HostRoute) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
