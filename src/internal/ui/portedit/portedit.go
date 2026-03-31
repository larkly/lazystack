package portedit

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
	fieldAllowPairs = 1
	fieldSecGroups  = 2
	fieldAdmin      = 3
	fieldPortSec    = 4
	fieldSubmit     = 5
	fieldCancel     = 6
	numFields       = 7
)

var toggleOpts = []string{"Enabled", "Disabled"}

type portUpdatedMsg struct{ name string }
type portUpdateErrMsg struct{ err error }

// Model is the port edit modal.
type Model struct {
	Active         bool
	client         *gophercloud.ServiceClient
	port           network.Port
	sgNames        map[string]string // SG ID → name
	nameInput      textinput.Model
	allowPairInput textinput.Model
	sgInput        textinput.Model
	adminState     int // 0=Enabled, 1=Disabled
	portSecurity   int // 0=Enabled, 1=Disabled
	focusField     int
	submitting     bool
	spinner        spinner.Model
	err            string
	width          int
	height         int
}

// New creates a port edit modal.
func New(client *gophercloud.ServiceClient, port network.Port, sgNames map[string]string) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "port name"
	ni.CharLimit = 255
	ni.SetWidth(40)
	ni.SetValue(port.Name)
	ni.Focus()

	// Format allowed address pairs
	var apStrs []string
	for _, ap := range port.AllowedAddressPairs {
		if ap.MACAddress != "" {
			apStrs = append(apStrs, ap.IPAddress+","+ap.MACAddress)
		} else {
			apStrs = append(apStrs, ap.IPAddress)
		}
	}
	ai := textinput.New()
	ai.Prompt = ""
	ai.Placeholder = "ip;ip,mac (semicolons between entries)"
	ai.CharLimit = 500
	ai.SetWidth(40)
	ai.SetValue(strings.Join(apStrs, "; "))

	// Format security groups as names
	var sgStrs []string
	for _, sgID := range port.SecurityGroups {
		if name, ok := sgNames[sgID]; ok {
			sgStrs = append(sgStrs, name)
		} else {
			sgStrs = append(sgStrs, sgID)
		}
	}
	si := textinput.New()
	si.Prompt = ""
	si.Placeholder = "security group names (comma-separated)"
	si.CharLimit = 500
	si.SetWidth(40)
	si.SetValue(strings.Join(sgStrs, ", "))

	admin := 0
	if !port.AdminStateUp {
		admin = 1
	}
	ps := 0
	if !port.PortSecurityEnabled {
		ps = 1
	}

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:         true,
		client:         client,
		port:           port,
		sgNames:        sgNames,
		nameInput:      ni,
		allowPairInput: ai,
		sgInput:        si,
		adminState:     admin,
		portSecurity:   ps,
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
	case portUpdatedMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Updated port", Name: msg.name}
		}
	case portUpdateErrMsg:
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
	case fieldName, fieldAllowPairs, fieldSecGroups:
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
	case fieldAllowPairs:
		m.allowPairInput, cmd = m.allowPairInput.Update(msg)
	case fieldSecGroups:
		m.sgInput, cmd = m.sgInput.Update(msg)
	}
	return m, cmd
}

func (m *Model) updateFocus() {
	m.nameInput.Blur()
	m.allowPairInput.Blur()
	m.sgInput.Blur()
	switch m.focusField {
	case fieldName:
		m.nameInput.Focus()
	case fieldAllowPairs:
		m.allowPairInput.Focus()
	case fieldSecGroups:
		m.sgInput.Focus()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	opts := network.PortUpdateOpts{}

	name := strings.TrimSpace(m.nameInput.Value())
	if name != m.port.Name {
		opts.Name = &name
	}

	adminUp := m.adminState == 0
	if adminUp != m.port.AdminStateUp {
		opts.AdminStateUp = &adminUp
	}

	pse := m.portSecurity == 0
	if pse != m.port.PortSecurityEnabled {
		opts.PortSecurityEnabled = &pse
	}

	// Parse allowed address pairs
	apRaw := strings.TrimSpace(m.allowPairInput.Value())
	pairs, err := parseAddressPairs(apRaw)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if !addressPairsEqual(pairs, m.port.AllowedAddressPairs) {
		opts.AllowedAddressPairs = &pairs
	}

	// Parse security groups - resolve names to IDs
	sgRaw := strings.TrimSpace(m.sgInput.Value())
	sgIDs, err := m.resolveSecurityGroups(sgRaw)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if !stringSliceEqual(sgIDs, m.port.SecurityGroups) {
		opts.SecurityGroups = &sgIDs
	}

	m.submitting = true
	m.err = ""
	client := m.client
	portID := m.port.ID
	displayName := name
	if displayName == "" {
		displayName = portID[:8]
	}

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		shared.Debugf("[portedit] updating port %s", portID)
		err := network.UpdatePort(context.Background(), client, portID, opts)
		if err != nil {
			shared.Debugf("[portedit] error: %v", err)
			return portUpdateErrMsg{err: err}
		}
		shared.Debugf("[portedit] updated port %s", portID)
		return portUpdatedMsg{name: displayName}
	})
}

func (m Model) resolveSecurityGroups(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	// Build reverse map: name → ID
	nameToID := make(map[string]string)
	for id, name := range m.sgNames {
		nameToID[name] = id
	}
	var ids []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if id, ok := nameToID[part]; ok {
			ids = append(ids, id)
		} else {
			// Assume it's an ID
			ids = append(ids, part)
		}
	}
	return ids, nil
}

func parseAddressPairs(raw string) ([]network.AddressPair, error) {
	if raw == "" {
		return nil, nil
	}
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

func addressPairsEqual(a, b []network.AddressPair) bool {
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

// View renders the modal.
func (m Model) View() string {
	// Show port IPs and MAC in title area
	var subtitleParts []string
	for _, ip := range m.port.FixedIPs {
		subtitleParts = append(subtitleParts, ip.IPAddress)
	}
	subtitleParts = append(subtitleParts, m.port.MACAddress)
	subtitle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(
		" (" + strings.Join(subtitleParts, " / ") + ")")
	title := shared.StyleModalTitle.Render("Edit Port") + subtitle

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
		{"Allow Pairs", m.allowPairInput.View(), m.focusField == fieldAllowPairs},
		{"Sec Groups", m.sgInput.View(), m.focusField == fieldSecGroups},
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
	body.WriteString(shared.StyleHelp.Render("  allow pairs: ip;ip,mac (semicolons between entries)"))

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
