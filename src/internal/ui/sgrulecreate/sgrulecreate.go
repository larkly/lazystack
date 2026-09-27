package sgrulecreate

import (
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
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

// Protocols whose rules take a port range, and protocols whose rules take an
// ICMP type (port_range_min) and code (port_range_max) instead. Neutron
// accepts names and IANA numbers.
var (
	portProtocols = []string{"tcp", "udp", "sctp", "dccp", "udplite", "6", "17", "33", "132", "136"}
	icmpProtocols = []string{"icmp", "ipv6-icmp", "icmpv6", "1", "58"}
)

type ruleCreatedMsg struct{}
type ruleCreateErrMsg struct{ err error }
type ruleLoadedMsg struct{ spec *network.SecurityRuleSpec }
type ruleLoadErrMsg struct{ err error }

// ruleReplaceErrMsg reports that the replacement rule was created but the
// original could not be deleted. rollbackErr is nil when the replacement
// was removed again, leaving the security group unchanged.
type ruleReplaceErrMsg struct {
	oldID, newID string
	deleteErr    error
	rollbackErr  error
}

// Model is the security group rule create/edit form modal.
type Model struct {
	Active bool
	client *gophercloud.ServiceClient
	sgID   string
	sgName string

	protocols         []string // options offered; edit mode may add the rule's own
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

	// Edit mode: Neutron rules are immutable, so an edit creates the
	// replacement first and deletes the original only afterwards.
	editMode    bool
	oldRuleID   string
	loadingRule bool
	blocked     string // why this rule cannot be edited here, if anything
	original    *network.SecurityRuleSpec

	// Remote selectors that the form keeps but does not edit.
	remoteGroupID        string
	remoteAddressGroupID string
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
		protocols:         slices.Clone(protocols),
		selectedDirection: 0, // ingress
		selectedEtherType: 0, // IPv4
		selectedProtocol:  0, // tcp
		portMinInput:      pmin,
		portMaxInput:      pmax,
		remoteIPInput:     rip,
		spinner:           s,
	}
}

// NewEdit creates a rule edit form for an existing rule. The form is
// pre-filled from the list data, but the full rule (including null port
// bounds and remote address groups) is re-read in Init before it can be
// submitted.
func NewEdit(client *gophercloud.ServiceClient, sgID, sgName string, rule network.SecurityRule) Model {
	m := New(client, sgID, sgName)
	m.editMode = true
	m.oldRuleID = rule.ID
	m.loadingRule = true
	spec := network.SecurityRuleSpec{
		ID:                   rule.ID,
		SecGroupID:           sgID,
		Direction:            rule.Direction,
		EtherType:            rule.EtherType,
		Protocol:             rule.Protocol,
		RemoteIPPrefix:       rule.RemoteIPPrefix,
		RemoteGroupID:        rule.RemoteGroupID,
		RemoteAddressGroupID: rule.RemoteAddressGroupID,
	}
	if rule.PortRangeMin > 0 {
		v := rule.PortRangeMin
		spec.PortRangeMin = &v
	}
	if rule.PortRangeMax > 0 {
		v := rule.PortRangeMax
		spec.PortRangeMax = &v
	}
	m.fill(spec)
	return m
}

// fill pre-fills the form from spec and records anything the form cannot
// represent in m.blocked.
func (m *Model) fill(spec network.SecurityRuleSpec) {
	m.blocked = ""
	if i := slices.Index(directions, spec.Direction); i >= 0 {
		m.selectedDirection = i
	} else {
		m.blocked = fmt.Sprintf("unsupported direction %q", spec.Direction)
	}
	if i := slices.Index(etherTypes, spec.EtherType); i >= 0 {
		m.selectedEtherType = i
	} else {
		m.blocked = fmt.Sprintf("unsupported ether type %q", spec.EtherType)
	}

	// Keep the rule's own protocol selectable and selected, verbatim, so an
	// unfamiliar protocol can never silently turn into tcp.
	proto := spec.Protocol
	if proto == "" {
		proto = "any"
	}
	m.protocols = slices.Clone(protocols)
	if !slices.Contains(m.protocols, proto) {
		m.protocols = append(m.protocols, proto)
	}
	m.selectedProtocol = slices.Index(m.protocols, proto)

	m.portMinInput.SetValue(formatBound(spec.PortRangeMin))
	m.portMaxInput.SetValue(formatBound(spec.PortRangeMax))
	if protocolKind(proto) == kindNone && (spec.PortRangeMin != nil || spec.PortRangeMax != nil) {
		m.blocked = fmt.Sprintf("the rule has port values for protocol %q, which this form cannot edit", proto)
	}

	m.remoteIPInput.SetValue(spec.RemoteIPPrefix)
	m.remoteGroupID = spec.RemoteGroupID
	m.remoteAddressGroupID = spec.RemoteAddressGroupID
	remotes := 0
	for _, r := range []string{spec.RemoteIPPrefix, spec.RemoteGroupID, spec.RemoteAddressGroupID} {
		if r != "" {
			remotes++
		}
	}
	if remotes > 1 {
		m.blocked = "the rule combines several remote selectors"
	}
}

func formatBound(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

type protoKind int

const (
	kindNone protoKind = iota
	kindPorts
	kindICMP
)

// currentProtocol returns the selected protocol, or "any" when the
// protocol list is not populated.
func (m Model) currentProtocol() string {
	if m.selectedProtocol < 0 || m.selectedProtocol >= len(m.protocols) {
		return "any"
	}
	return m.protocols[m.selectedProtocol]
}

func protocolKind(proto string) protoKind {
	p := strings.ToLower(proto)
	switch {
	case slices.Contains(portProtocols, p):
		return kindPorts
	case slices.Contains(icmpProtocols, p):
		return kindICMP
	}
	return kindNone
}

// Init loads the full rule in edit mode.
func (m Model) Init() tea.Cmd {
	if !m.editMode {
		return nil
	}
	client := m.client
	id := m.oldRuleID
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		spec, err := network.GetSecurityRuleSpec(ctx, client, id)
		if err != nil {
			return ruleLoadErrMsg{err: err}
		}
		return ruleLoadedMsg{spec: spec}
	})
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ruleLoadedMsg:
		m.loadingRule = false
		spec := *msg.spec
		spec.SecGroupID = m.sgID
		m.original = &spec
		m.fill(spec)
		return m, nil
	case ruleLoadErrMsg:
		m.loadingRule = false
		m.blocked = "could not load the rule details (" + msg.err.Error() + "); nothing was changed"
		return m, nil
	case ruleCreatedMsg:
		m.submitting = false
		m.Active = false
		action := "Created rule in"
		if m.editMode {
			action = "Updated rule in"
		}
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: action, Name: m.sgName}
		}
	case ruleCreateErrMsg:
		m.submitting = false
		m.err = msg.err.Error()
		return m, nil
	case ruleReplaceErrMsg:
		m.submitting = false
		if msg.rollbackErr == nil {
			m.err = fmt.Sprintf("Could not delete the original rule %s (%v). The replacement rule %s was removed again, so the rule is unchanged.",
				msg.oldID, msg.deleteErr, msg.newID)
			return m, nil
		}
		// Both rules now exist; close so the list refreshes and say so.
		m.Active = false
		err := fmt.Errorf("created replacement rule %s but could not delete the original rule %s (%v) or roll back the replacement (%v); both rules are active, delete one manually",
			msg.newID, msg.oldID, msg.deleteErr, msg.rollbackErr)
		return m, func() tea.Msg {
			return shared.ResourceActionErrMsg{Action: "Update rule in", Name: m.sgName, Err: err}
		}
	case spinner.TickMsg:
		if m.submitting || m.loadingRule {
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
			m.selectedProtocol = (m.selectedProtocol + 1) % len(m.protocols)
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
			m.selectedProtocol = (m.selectedProtocol - 1 + len(m.protocols)) % len(m.protocols)
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

// parseBound parses an optional port/ICMP bound; empty means unset.
func parseBound(raw, label string, lo, hi int) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < lo || v > hi {
		return nil, fmt.Errorf("%s must be %d-%d", label, lo, hi)
	}
	return &v, nil
}

// buildSpec turns the form into a rule definition, or explains why the
// input cannot be submitted.
func (m Model) buildSpec() (network.SecurityRuleSpec, error) {
	spec := network.SecurityRuleSpec{
		SecGroupID:           m.sgID,
		Direction:            directions[m.selectedDirection],
		EtherType:            etherTypes[m.selectedEtherType],
		RemoteGroupID:        m.remoteGroupID,
		RemoteAddressGroupID: m.remoteAddressGroupID,
	}
	if m.original != nil {
		spec.Description = m.original.Description
	}
	proto := m.currentProtocol()
	if proto != "any" {
		spec.Protocol = proto
	}

	var err error
	minRaw, maxRaw := m.portMinInput.Value(), m.portMaxInput.Value()
	switch protocolKind(proto) {
	case kindPorts:
		if spec.PortRangeMin, err = parseBound(minRaw, "Port min", 1, 65535); err != nil {
			return spec, err
		}
		if spec.PortRangeMax, err = parseBound(maxRaw, "Port max", 1, 65535); err != nil {
			return spec, err
		}
		// A single bound means a single port.
		if spec.PortRangeMin != nil && spec.PortRangeMax == nil {
			spec.PortRangeMax = spec.PortRangeMin
		}
		if spec.PortRangeMax != nil && spec.PortRangeMin == nil {
			spec.PortRangeMin = spec.PortRangeMax
		}
		if spec.PortRangeMin != nil && *spec.PortRangeMin > *spec.PortRangeMax {
			return spec, fmt.Errorf("port min must not exceed port max")
		}
	case kindICMP:
		// For ICMP the bounds are type and code; empty means any, and 0 is
		// a real value (e.g. type 0 echo reply).
		if spec.PortRangeMin, err = parseBound(minRaw, "ICMP type", 0, 255); err != nil {
			return spec, err
		}
		if spec.PortRangeMax, err = parseBound(maxRaw, "ICMP code", 0, 255); err != nil {
			return spec, err
		}
		if spec.PortRangeMax != nil && spec.PortRangeMin == nil {
			return spec, fmt.Errorf("an ICMP code requires an ICMP type")
		}
	default:
		if strings.TrimSpace(minRaw) != "" || strings.TrimSpace(maxRaw) != "" {
			return spec, fmt.Errorf("ports only apply to port-based protocols such as tcp, udp or sctp; clear them for %q", proto)
		}
	}

	remoteIP := strings.TrimSpace(m.remoteIPInput.Value())
	if remoteIP != "" {
		if spec.RemoteGroupID != "" || spec.RemoteAddressGroupID != "" {
			return spec, fmt.Errorf("this rule is restricted to a remote %s; a remote IP cannot be combined with it (delete the rule and add a new one to change the remote)", m.remoteKind())
		}
		addr, err := netip.ParsePrefix(remoteIP)
		if err != nil {
			a, aerr := netip.ParseAddr(remoteIP)
			if aerr != nil {
				return spec, fmt.Errorf("invalid remote IP %q", remoteIP)
			}
			addr = netip.PrefixFrom(a, a.BitLen())
		}
		if addr.Addr().Is4() != (spec.EtherType == "IPv4") {
			return spec, fmt.Errorf("remote IP %s does not match ether type %s", remoteIP, spec.EtherType)
		}
		spec.RemoteIPPrefix = remoteIP
	}
	return spec, nil
}

func (m Model) remoteKind() string {
	if m.remoteGroupID != "" {
		return "security group (" + m.remoteGroupID + ")"
	}
	return "address group (" + m.remoteAddressGroupID + ")"
}

func equalBound(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameRule(a, b network.SecurityRuleSpec) bool {
	return a.Direction == b.Direction && a.EtherType == b.EtherType && a.Protocol == b.Protocol &&
		equalBound(a.PortRangeMin, b.PortRangeMin) && equalBound(a.PortRangeMax, b.PortRangeMax) &&
		a.RemoteIPPrefix == b.RemoteIPPrefix && a.RemoteGroupID == b.RemoteGroupID &&
		a.RemoteAddressGroupID == b.RemoteAddressGroupID
}

func (m Model) submit() (Model, tea.Cmd) {
	if m.loadingRule {
		m.err = "Rule details are still loading"
		return m, nil
	}
	if m.blocked != "" {
		m.err = "This rule cannot be edited here: " + m.blocked
		return m, nil
	}
	spec, err := m.buildSpec()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if m.editMode && m.original != nil && sameRule(spec, *m.original) {
		// Nothing to do; recreating an identical rule would also be rejected
		// by Neutron as a duplicate.
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "No changes to rule in", Name: m.sgName}
		}
	}

	m.submitting = true
	m.err = ""
	client := m.client
	editMode := m.editMode
	oldRuleID := m.oldRuleID
	sgName := m.sgName
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		if editMode {
			shared.Debugf("[sgrulecreate] editing rule in %q (replacing %s)", sgName, oldRuleID)
		} else {
			shared.Debugf("[sgrulecreate] creating rule in %q (%s %s %q)", sgName, spec.Direction, spec.EtherType, spec.Protocol)
		}
		newID, err := network.CreateSecurityRuleSpec(ctx, client, spec)
		if err != nil {
			shared.Debugf("[sgrulecreate] error creating rule in %q: %v", sgName, err)
			return ruleCreateErrMsg{err: err}
		}
		// In edit mode, delete the old rule only after the new one exists.
		if editMode && oldRuleID != "" {
			delErr := network.DeleteSecurityGroupRule(ctx, client, oldRuleID)
			if delErr != nil && !gophercloud.ResponseCodeIs(delErr, http.StatusNotFound) {
				shared.Debugf("[sgrulecreate] could not delete original rule %s: %v; rolling back %s", oldRuleID, delErr, newID)
				// Fresh deadline: the delete may have failed by timing out.
				rbCtx, rbCancel := shared.RequestCtx()
				rbErr := network.DeleteSecurityGroupRule(rbCtx, client, newID)
				rbCancel()
				return ruleReplaceErrMsg{oldID: oldRuleID, newID: newID, deleteErr: delErr, rollbackErr: rbErr}
			}
			shared.Debugf("[sgrulecreate] edited rule in %q (%s -> %s)", sgName, oldRuleID, newID)
		} else {
			shared.Debugf("[sgrulecreate] created rule %s in %q", newID, sgName)
		}
		return ruleCreatedMsg{}
	})
}

// View renders the rule create form modal.
func (m Model) View() string {
	titleText := "Add Rule to " + m.sgName
	if m.editMode {
		titleText = "Edit Rule in " + m.sgName
	}
	title := shared.StyleModalTitle.Render(titleText)

	var body strings.Builder

	if m.submitting {
		body.WriteString(m.spinner.View() + " Saving rule...")
		content := title + "\n\n" + body.String()
		return m.renderModal(content)
	}

	if m.loadingRule {
		body.WriteString(m.spinner.View() + " Loading rule details...\n\n")
	}
	if m.blocked != "" {
		body.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("⚠ This rule cannot be edited here: "+m.blocked) + "\n\n")
	} else if m.err != "" {
		body.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("⚠ "+m.err) + "\n\n")
	}

	type field struct {
		label   string
		value   string
		focused bool
	}

	minLabel, maxLabel := "Port Min", "Port Max"
	minInput, maxInput := m.portMinInput, m.portMaxInput
	if protocolKind(m.currentProtocol()) == kindICMP {
		minLabel, maxLabel = "ICMP Type", "ICMP Code"
		minInput.Placeholder, maxInput.Placeholder = "any", "any"
	}
	fields := []field{
		{"Direction", m.cycleDisplay(directions, m.selectedDirection), m.focusField == fieldDirection},
		{"EtherType", m.cycleDisplay(etherTypes, m.selectedEtherType), m.focusField == fieldEtherType},
		{"Protocol", m.cycleDisplay(m.protocols, m.selectedProtocol), m.focusField == fieldProtocol},
		{minLabel, minInput.View(), m.focusField == fieldPortMin},
		{maxLabel, maxInput.View(), m.focusField == fieldPortMax},
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
	if m.remoteGroupID != "" || m.remoteAddressGroupID != "" {
		label := lipgloss.NewStyle().Width(12).Foreground(shared.ColorSecondary).Render("Remote")
		body.WriteString("  " + label + " " + shared.StyleHelp.Render(m.remoteKind()+", kept") + "\n")
	}

	body.WriteString("\n")
	submitStyle := shared.StyleButton
	cancelStyle := shared.StyleButton
	if m.focusField == fieldSubmit {
		submitStyle = shared.StyleButtonSubmit
	}
	if m.focusField == fieldCancel {
		cancelStyle = shared.StyleButtonCancel
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
	modalWidth := 72
	if m.width > 0 && m.width < 82 {
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
