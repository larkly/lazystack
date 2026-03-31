package portcreate

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
	fieldFixedIPs   = 1
	fieldSecGroups  = 2
	fieldAllowPairs = 3
	fieldAdmin      = 4
	fieldPortSec    = 5
	fieldSubmit     = 6
	fieldCancel     = 7
	numFields       = 8
)

var (
	toggleOpts = []string{"Enabled", "Disabled"}
)

type portCreatedMsg struct{ name string }
type portCreateErrMsg struct{ err error }

// Model is the port create modal.
type Model struct {
	Active         bool
	client         *gophercloud.ServiceClient
	networkID      string
	networkName    string
	subnets        []network.Subnet
	secGroups      []network.SecurityGroup
	nameInput      textinput.Model
	fixedIPInput   textinput.Model
	sgInput        textinput.Model
	allowPairInput textinput.Model
	adminState     int // 0=Enabled, 1=Disabled
	portSecurity   int // 0=Enabled, 1=Disabled
	focusField     int
	submitting     bool
	spinner        spinner.Model
	err            string
	width          int
	height         int
}

// New creates a port create modal.
func New(client *gophercloud.ServiceClient, networkID, networkName string, subnets []network.Subnet, secGroups []network.SecurityGroup) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "port name (optional)"
	ni.CharLimit = 255
	ni.SetWidth(40)
	ni.Focus()

	fi := textinput.New()
	fi.Prompt = ""
	fi.Placeholder = "subnet:ip, subnet:ip (or just ip)"
	fi.CharLimit = 500
	fi.SetWidth(40)

	si := textinput.New()
	si.Prompt = ""
	si.Placeholder = "security group names (comma-separated)"
	si.CharLimit = 500
	si.SetWidth(40)

	ai := textinput.New()
	ai.Prompt = ""
	ai.Placeholder = "ip or ip,mac (comma-separated pairs)"
	ai.CharLimit = 500
	ai.SetWidth(40)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:         true,
		client:         client,
		networkID:      networkID,
		networkName:    networkName,
		subnets:        subnets,
		secGroups:      secGroups,
		nameInput:      ni,
		fixedIPInput:   fi,
		sgInput:        si,
		allowPairInput: ai,
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
	case portCreatedMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Created port on", Name: m.networkName}
		}
	case portCreateErrMsg:
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
	case fieldName, fieldFixedIPs, fieldSecGroups, fieldAllowPairs:
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
		case fieldAdmin:
			m.adminState = (m.adminState + 1) % 2
			return m, nil
		case fieldPortSec:
			m.portSecurity = (m.portSecurity + 1) % 2
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
		case fieldAdmin, fieldPortSec:
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
	case fieldFixedIPs:
		m.fixedIPInput, cmd = m.fixedIPInput.Update(msg)
	case fieldSecGroups:
		m.sgInput, cmd = m.sgInput.Update(msg)
	case fieldAllowPairs:
		m.allowPairInput, cmd = m.allowPairInput.Update(msg)
	}
	return m, cmd
}

func (m *Model) updateFocus() {
	m.nameInput.Blur()
	m.fixedIPInput.Blur()
	m.sgInput.Blur()
	m.allowPairInput.Blur()
	switch m.focusField {
	case fieldName:
		m.nameInput.Focus()
	case fieldFixedIPs:
		m.fixedIPInput.Focus()
	case fieldSecGroups:
		m.sgInput.Focus()
	case fieldAllowPairs:
		m.allowPairInput.Focus()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	opts := network.PortCreateOpts{
		NetworkID:    m.networkID,
		Name:         strings.TrimSpace(m.nameInput.Value()),
		AdminStateUp: m.adminState == 0,
	}

	pse := m.portSecurity == 0
	opts.PortSecurityEnabled = &pse

	// Parse fixed IPs
	fipRaw := strings.TrimSpace(m.fixedIPInput.Value())
	if fipRaw != "" {
		fips, err := m.parseFixedIPs(fipRaw)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		opts.FixedIPs = fips
	}

	// Parse security groups
	sgRaw := strings.TrimSpace(m.sgInput.Value())
	if sgRaw != "" {
		sgs, err := m.resolveSecurityGroups(sgRaw)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		opts.SecurityGroups = sgs
	}

	// Parse allowed address pairs
	apRaw := strings.TrimSpace(m.allowPairInput.Value())
	if apRaw != "" {
		pairs, err := parseAddressPairs(apRaw)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		opts.AllowedAddressPairs = pairs
	}

	m.submitting = true
	m.err = ""
	client := m.client
	name := opts.Name
	if name == "" {
		name = m.networkName
	}

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		shared.Debugf("[portcreate] creating port on network %s", m.networkID)
		_, err := network.CreatePortFull(context.Background(), client, opts)
		if err != nil {
			shared.Debugf("[portcreate] error: %v", err)
			return portCreateErrMsg{err: err}
		}
		shared.Debugf("[portcreate] created port on network %s", m.networkID)
		return portCreatedMsg{name: name}
	})
}

// parseFixedIPs parses "subnet:ip, subnet:ip" or just "ip" format.
func (m Model) parseFixedIPs(raw string) ([]network.FixedIP, error) {
	var result []network.FixedIP
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if idx := strings.Index(part, ":"); idx >= 0 {
			subnetName := strings.TrimSpace(part[:idx])
			ip := strings.TrimSpace(part[idx+1:])
			subnetID := m.resolveSubnet(subnetName)
			if subnetID == "" {
				return nil, fmt.Errorf("unknown subnet %q", subnetName)
			}
			result = append(result, network.FixedIP{SubnetID: subnetID, IPAddress: ip})
		} else {
			// Just an IP — need exactly one subnet on the network
			if len(m.subnets) == 0 {
				return nil, fmt.Errorf("no subnets on network to assign IP %q", part)
			}
			if len(m.subnets) > 1 {
				return nil, fmt.Errorf("multiple subnets on network, use subnet:ip format")
			}
			result = append(result, network.FixedIP{SubnetID: m.subnets[0].ID, IPAddress: part})
		}
	}
	return result, nil
}

func (m Model) resolveSubnet(name string) string {
	for _, s := range m.subnets {
		if s.Name == name || s.ID == name || (len(s.ID) >= len(name) && s.ID[:len(name)] == name) {
			return s.ID
		}
	}
	return ""
}

func (m Model) resolveSecurityGroups(raw string) ([]string, error) {
	var ids []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		found := false
		for _, sg := range m.secGroups {
			if sg.Name == part || sg.ID == part || (len(sg.ID) >= len(part) && sg.ID[:len(part)] == part) {
				ids = append(ids, sg.ID)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown security group %q", part)
		}
	}
	return ids, nil
}

// parseAddressPairs parses "ip" or "ip,mac" entries separated by semicolons or commas.
// Format: "10.0.0.100; 10.0.0.200,fa:16:3e:xx:xx:xx"
func parseAddressPairs(raw string) ([]network.AddressPair, error) {
	var result []network.AddressPair
	// Use semicolons as primary delimiter since IPs may contain commas in ip,mac notation
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if idx := strings.Index(part, ","); idx >= 0 {
			ip := strings.TrimSpace(part[:idx])
			mac := strings.TrimSpace(part[idx+1:])
			result = append(result, network.AddressPair{IPAddress: ip, MACAddress: mac})
		} else {
			result = append(result, network.AddressPair{IPAddress: part})
		}
	}
	return result, nil
}

// View renders the modal.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Create Port on " + m.networkName)

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
		{"Fixed IPs", m.fixedIPInput.View(), m.focusField == fieldFixedIPs},
		{"Sec Groups", m.sgInput.View(), m.focusField == fieldSecGroups},
		{"Allow Pairs", m.allowPairInput.View(), m.focusField == fieldAllowPairs},
		{"Admin State", cycleDisplay(toggleOpts, m.adminState), m.focusField == fieldAdmin},
		{"Port Security", cycleDisplay(toggleOpts, m.portSecurity), m.focusField == fieldPortSec},
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
	body.WriteString("\n")
	body.WriteString(shared.StyleHelp.Render("  fixed IPs: subnet:ip • allow pairs: ip;ip,mac (semicolons)"))

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
