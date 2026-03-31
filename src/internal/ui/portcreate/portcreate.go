package portcreate

import (
	"context"
	"fmt"
	"sort"
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

var toggleOpts = []string{"Enabled", "Disabled"}

type portCreatedMsg struct{ name string }
type portCreateErrMsg struct{ err error }
type sgLoadedMsg struct{ sgs []network.SecurityGroup }
type sgLoadErrMsg struct{ err error }

// Model is the port create modal.
type Model struct {
	Active         bool
	client         *gophercloud.ServiceClient
	networkID      string
	networkName    string
	subnets        []network.Subnet
	secGroups      []network.SecurityGroup
	selectedSGs    map[int]bool
	nameInput      textinput.Model
	fixedIPInput   textinput.Model
	allowPairInput textinput.Model
	adminState     int // 0=Enabled, 1=Disabled
	portSecurity   int // 0=Enabled, 1=Disabled
	focusField     int
	submitting     bool
	loadingSGs     bool
	spinner        spinner.Model
	err            string
	width          int
	height         int

	// Inline SG picker state
	sgPickerOpen   bool
	sgPickerCursor int
	sgPickerFilter textinput.Model
}

// New creates a port create modal.
func New(client *gophercloud.ServiceClient, networkID, networkName string, subnets []network.Subnet) Model {
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

	ai := textinput.New()
	ai.Prompt = ""
	ai.Placeholder = "ip or ip,mac (semicolons between entries)"
	ai.CharLimit = 500
	ai.SetWidth(40)

	pf := textinput.New()
	pf.Prompt = "  🔍 "
	pf.Placeholder = "filter..."
	pf.CharLimit = 50
	pf.SetWidth(30)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:         true,
		client:         client,
		networkID:      networkID,
		networkName:    networkName,
		subnets:        subnets,
		selectedSGs:    make(map[int]bool),
		nameInput:      ni,
		fixedIPInput:   fi,
		allowPairInput: ai,
		sgPickerFilter: pf,
		loadingSGs:     true,
		spinner:        s,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	client := m.client
	return tea.Batch(textinput.Blink, m.spinner.Tick, func() tea.Msg {
		sgs, err := network.ListSecurityGroups(context.Background(), client)
		if err != nil {
			return sgLoadErrMsg{err: err}
		}
		return sgLoadedMsg{sgs: sgs}
	})
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sgLoadedMsg:
		m.loadingSGs = false
		m.secGroups = msg.sgs
		for i, sg := range m.secGroups {
			if sg.Name == "default" {
				m.selectedSGs[i] = true
				break
			}
		}
		return m, nil
	case sgLoadErrMsg:
		m.loadingSGs = false
		m.err = "Failed to load security groups: " + msg.err.Error()
		return m, nil
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
		if m.submitting || m.loadingSGs {
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
		if m.sgPickerOpen {
			return m.updateSGPicker(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) isTextInput() bool {
	switch m.focusField {
	case fieldName, fieldFixedIPs, fieldAllowPairs:
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
		case fieldSecGroups:
			m.sgPickerOpen = true
			m.sgPickerCursor = 0
			m.sgPickerFilter.SetValue("")
			m.sgPickerFilter.Focus()
			return m, nil
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

func (m Model) updateSGPicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	filtered := m.filteredSGs()

	switch msg.String() {
	case "esc":
		m.sgPickerOpen = false
		m.sgPickerFilter.Blur()
		return m, nil
	case "enter":
		m.sgPickerOpen = false
		m.sgPickerFilter.Blur()
		m.focusField++
		m.updateFocus()
		return m, nil
	case "space":
		if len(filtered) > 0 && m.sgPickerCursor < len(filtered) {
			idx := filtered[m.sgPickerCursor].id
			if m.selectedSGs[idx] {
				delete(m.selectedSGs, idx)
			} else {
				m.selectedSGs[idx] = true
			}
		}
		return m, nil
	case "up":
		if m.sgPickerCursor > 0 {
			m.sgPickerCursor--
		}
		return m, nil
	case "down":
		if m.sgPickerCursor < len(filtered)-1 {
			m.sgPickerCursor++
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.sgPickerFilter, cmd = m.sgPickerFilter.Update(msg)
	// Reset cursor when filter changes
	m.sgPickerCursor = 0
	return m, cmd
}

type sgItem struct {
	id   int
	name string
	desc string
}

func (m Model) filteredSGs() []sgItem {
	q := strings.ToLower(m.sgPickerFilter.Value())
	var items []sgItem
	for i, sg := range m.secGroups {
		if q == "" || strings.Contains(strings.ToLower(sg.Name), q) ||
			strings.Contains(strings.ToLower(sg.Description), q) {
			items = append(items, sgItem{id: i, name: sg.Name, desc: sg.Description})
		}
	}
	return items
}

func (m Model) routeToInput(msg tea.KeyMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focusField {
	case fieldName:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case fieldFixedIPs:
		m.fixedIPInput, cmd = m.fixedIPInput.Update(msg)
	case fieldAllowPairs:
		m.allowPairInput, cmd = m.allowPairInput.Update(msg)
	}
	return m, cmd
}

func (m *Model) updateFocus() {
	m.nameInput.Blur()
	m.fixedIPInput.Blur()
	m.allowPairInput.Blur()
	switch m.focusField {
	case fieldName:
		m.nameInput.Focus()
	case fieldFixedIPs:
		m.fixedIPInput.Focus()
	case fieldAllowPairs:
		m.allowPairInput.Focus()
	}
}

func (m Model) sortedSGIndices() []int {
	indices := make([]int, 0, len(m.selectedSGs))
	for idx := range m.selectedSGs {
		if idx < len(m.secGroups) {
			indices = append(indices, idx)
		}
	}
	sort.Ints(indices)
	return indices
}

func (m Model) sgDisplayValue() string {
	indices := m.sortedSGIndices()
	if len(indices) == 0 {
		return lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("none selected")
	}
	var names []string
	for _, idx := range indices {
		names = append(names, m.secGroups[idx].Name)
	}
	return strings.Join(names, ", ")
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

	// Collect selected security groups (empty slice = no SGs, nil = server default)
	indices := m.sortedSGIndices()
	sgIDs := make([]string, 0, len(indices))
	for _, idx := range indices {
		sgIDs = append(sgIDs, m.secGroups[idx].ID)
	}
	opts.SecurityGroups = sgIDs

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

func parseAddressPairs(raw string) ([]network.AddressPair, error) {
	var result []network.AddressPair
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

	sgValue := m.sgDisplayValue()
	if m.loadingSGs {
		sgValue = m.spinner.View() + " Loading..."
	}

	fields := []field{
		{"Name", m.nameInput.View(), m.focusField == fieldName},
		{"Fixed IPs", m.fixedIPInput.View(), m.focusField == fieldFixedIPs},
		{"Sec Groups", sgValue, m.focusField == fieldSecGroups},
		{"Allow Pairs", m.allowPairInput.View(), m.focusField == fieldAllowPairs},
		{"Admin State", cycleDisplay(toggleOpts, m.adminState), m.focusField == fieldAdmin},
		{"Port Security", cycleDisplay(toggleOpts, m.portSecurity), m.focusField == fieldPortSec},
	}

	for i, f := range fields {
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

		// Show inline SG picker
		if i == 2 && m.sgPickerOpen {
			body.WriteString(m.renderSGPicker())
		}
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
	if m.sgPickerOpen {
		body.WriteString(shared.StyleHelp.Render("  ↑↓ navigate • space toggle • enter confirm • esc close • type to filter"))
	} else {
		body.WriteString(shared.StyleHelp.Render("  tab/↑↓ fields • ←→ cycle • ctrl+s submit • esc cancel"))
		body.WriteString("\n")
		body.WriteString(shared.StyleHelp.Render("  fixed IPs: subnet:ip • allow pairs: ip;ip,mac (semicolons)"))
	}

	content := title + "\n\n" + body.String()
	return m.renderModal(content)
}

func (m Model) renderSGPicker() string {
	var b strings.Builder
	filtered := m.filteredSGs()

	b.WriteString("      " + m.sgPickerFilter.View() + "\n")

	maxShow := 8
	if len(filtered) < maxShow {
		maxShow = len(filtered)
	}

	start := 0
	if m.sgPickerCursor >= maxShow {
		start = m.sgPickerCursor - maxShow + 1
	}
	end := start + maxShow
	if end > len(filtered) {
		end = len(filtered)
	}

	for i := start; i < end; i++ {
		item := filtered[i]
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.sgPickerCursor {
			cursor = "▸ "
			style = lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
		}
		check := "○ "
		if m.selectedSGs[item.id] {
			check = "● "
		}
		desc := ""
		if item.desc != "" {
			desc = shared.StyleHelp.Render(" " + item.desc)
		}
		b.WriteString(fmt.Sprintf("      %s%s%s%s\n", cursor, check, style.Render(item.name), desc))
	}

	return b.String()
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
