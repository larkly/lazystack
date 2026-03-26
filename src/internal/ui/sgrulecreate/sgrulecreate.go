package sgrulecreate

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
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/security/rules"
)

const (
	fieldDirection = 0
	fieldEtherType = 1
	fieldProtocol  = 2
	fieldPortMin   = 3
	fieldPortMax   = 4
	fieldRemoteIP  = 5
	fieldSubmit    = 6
	fieldCancel    = 7
	numFields      = 8
)

var (
	directions = []string{"ingress", "egress"}
	etherTypes = []string{"IPv4", "IPv6"}
	protocols  = []string{"tcp", "udp", "icmp", "any"}
)

type ruleCreatedMsg struct{}
type ruleCreateErrMsg struct{ err error }

// Model is the security group rule create form modal.
type Model struct {
	Active    bool
	client    *gophercloud.ServiceClient
	sgID      string
	sgName    string

	selectedDirection int
	selectedEtherType int
	selectedProtocol  int
	portMinInput      textinput.Model
	portMaxInput      textinput.Model
	remoteIPInput     textinput.Model

	focusField int
	submitting bool
	spinner    spinner.Model
	err        string
	width      int
	height     int
}

// New creates a rule create form for the given security group.
func New(client *gophercloud.ServiceClient, sgID, sgName string) Model {
	pmin := textinput.New()
	pmin.Prompt = ""
	pmin.Placeholder = "port min"
	pmin.CharLimit = 5
	pmin.SetWidth(10)

	pmax := textinput.New()
	pmax.Prompt = ""
	pmax.Placeholder = "port max"
	pmax.CharLimit = 5
	pmax.SetWidth(10)

	rip := textinput.New()
	rip.Prompt = ""
	rip.Placeholder = "e.g. 0.0.0.0/0"
	rip.CharLimit = 43
	rip.SetWidth(25)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:            true,
		client:            client,
		sgID:              sgID,
		sgName:            sgName,
		selectedDirection: 0, // ingress
		selectedEtherType: 0, // IPv4
		selectedProtocol:  0, // tcp
		portMinInput:      pmin,
		portMaxInput:      pmax,
		remoteIPInput:     rip,
		spinner:           s,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ruleCreatedMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Created rule in", Name: m.sgName}
		}
	case ruleCreateErrMsg:
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
	return m.focusField == fieldPortMin || m.focusField == fieldPortMax || m.focusField == fieldRemoteIP
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
			case fieldPortMin:
				var cmd tea.Cmd
				m.portMinInput, cmd = m.portMinInput.Update(msg)
				return m, cmd
			case fieldPortMax:
				var cmd tea.Cmd
				m.portMaxInput, cmd = m.portMaxInput.Update(msg)
				return m, cmd
			case fieldRemoteIP:
				var cmd tea.Cmd
				m.remoteIPInput, cmd = m.remoteIPInput.Update(msg)
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
		case fieldDirection:
			m.selectedDirection = (m.selectedDirection + 1) % len(directions)
			return m, nil
		case fieldEtherType:
			m.selectedEtherType = (m.selectedEtherType + 1) % len(etherTypes)
			return m, nil
		case fieldProtocol:
			m.selectedProtocol = (m.selectedProtocol + 1) % len(protocols)
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
		case fieldDirection:
			m.selectedDirection = (m.selectedDirection - 1 + len(directions)) % len(directions)
			return m, nil
		case fieldEtherType:
			m.selectedEtherType = (m.selectedEtherType - 1 + len(etherTypes)) % len(etherTypes)
			return m, nil
		case fieldProtocol:
			m.selectedProtocol = (m.selectedProtocol - 1 + len(protocols)) % len(protocols)
			return m, nil
		case fieldCancel:
			m.focusField = fieldSubmit
			return m, nil
		case fieldSubmit:
			m.focusField = fieldCancel
			return m, nil
		}

	case key.Matches(msg, shared.Keys.Enter):
		switch m.focusField {
		case fieldDirection, fieldEtherType, fieldProtocol:
			m.focusField++
			m.updateFocus()
			return m, nil
		case fieldPortMin, fieldPortMax, fieldRemoteIP:
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
	case fieldPortMin:
		var cmd tea.Cmd
		m.portMinInput, cmd = m.portMinInput.Update(msg)
		return m, cmd
	case fieldPortMax:
		var cmd tea.Cmd
		m.portMaxInput, cmd = m.portMaxInput.Update(msg)
		return m, cmd
	case fieldRemoteIP:
		var cmd tea.Cmd
		m.remoteIPInput, cmd = m.remoteIPInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *Model) updateFocus() {
	if m.focusField == fieldPortMin {
		m.portMinInput.Focus()
	} else {
		m.portMinInput.Blur()
	}
	if m.focusField == fieldPortMax {
		m.portMaxInput.Focus()
	} else {
		m.portMaxInput.Blur()
	}
	if m.focusField == fieldRemoteIP {
		m.remoteIPInput.Focus()
	} else {
		m.remoteIPInput.Blur()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	dir := directions[m.selectedDirection]
	etherType := etherTypes[m.selectedEtherType]
	proto := protocols[m.selectedProtocol]

	var ruleDir rules.RuleDirection
	if dir == "ingress" {
		ruleDir = rules.DirIngress
	} else {
		ruleDir = rules.DirEgress
	}

	var ruleEther rules.RuleEtherType
	if etherType == "IPv4" {
		ruleEther = rules.EtherType4
	} else {
		ruleEther = rules.EtherType6
	}

	opts := rules.CreateOpts{
		SecGroupID:  m.sgID,
		Direction:   ruleDir,
		EtherType:   ruleEther,
	}

	if proto != "any" {
		opts.Protocol = rules.RuleProtocol(proto)
	}

	// Port range only applicable for tcp/udp
	if proto == "tcp" || proto == "udp" {
		minStr := strings.TrimSpace(m.portMinInput.Value())
		maxStr := strings.TrimSpace(m.portMaxInput.Value())
		if minStr != "" {
			min, err := strconv.Atoi(minStr)
			if err != nil || min < 1 || min > 65535 {
				m.err = "Port min must be 1-65535"
				return m, nil
			}
			opts.PortRangeMin = min
		}
		if maxStr != "" {
			max, err := strconv.Atoi(maxStr)
			if err != nil || max < 1 || max > 65535 {
				m.err = "Port max must be 1-65535"
				return m, nil
			}
			opts.PortRangeMax = max
		}
		// If only min provided, set max = min (single port)
		if opts.PortRangeMin > 0 && opts.PortRangeMax == 0 {
			opts.PortRangeMax = opts.PortRangeMin
		}
		if opts.PortRangeMax > 0 && opts.PortRangeMin == 0 {
			opts.PortRangeMin = opts.PortRangeMax
		}
	}

	remoteIP := strings.TrimSpace(m.remoteIPInput.Value())
	if remoteIP != "" {
		opts.RemoteIPPrefix = remoteIP
	}

	m.submitting = true
	m.err = ""
	client := m.client
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		_, err := network.CreateSecurityGroupRule(context.Background(), client, opts)
		if err != nil {
			return ruleCreateErrMsg{err: err}
		}
		return ruleCreatedMsg{}
	})
}

// View renders the rule create form modal.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Add Rule to " + m.sgName)

	var body strings.Builder

	if m.submitting {
		body.WriteString(m.spinner.View() + " Creating rule...")
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
		{"Direction", m.cycleDisplay(directions, m.selectedDirection), m.focusField == fieldDirection},
		{"EtherType", m.cycleDisplay(etherTypes, m.selectedEtherType), m.focusField == fieldEtherType},
		{"Protocol", m.cycleDisplay(protocols, m.selectedProtocol), m.focusField == fieldProtocol},
		{"Port Min", m.portMinInput.View(), m.focusField == fieldPortMin},
		{"Port Max", m.portMaxInput.View(), m.focusField == fieldPortMax},
		{"Remote IP", m.remoteIPInput.View(), m.focusField == fieldRemoteIP},
	}

	for _, f := range fields {
		cursor := "  "
		if f.focused {
			cursor = "▸ "
		}
		label := lipgloss.NewStyle().Width(12).Foreground(shared.ColorSecondary).Render(f.label)
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

func (m Model) cycleDisplay(options []string, selected int) string {
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
