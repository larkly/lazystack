package secgroupview

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/copypicker"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type focusPane int

const (
	FocusSelector focusPane = iota
	focusInfo
	FocusRules
	focusServers
	focusPorts
)

const focusPaneCount = 5
const narrowThreshold = 80

type sgLoadedMsg struct{ groups []network.SecurityGroup }
type sgErrMsg struct{ err error }
type detailLoadedMsg struct {
	sgID       string
	servers    []serverRef
	ports      []network.Port
	serverNames map[string]string
}
type detailErrMsg struct {
	sgID string
	err  error
}

type serverRef struct {
	ID     string
	Name   string
	Status string
}

// Model is the combined security group selector + detail view.
type Model struct {
	networkClient *gophercloud.ServiceClient
	computeClient *gophercloud.ServiceClient

	// Selector state
	groups         []network.SecurityGroup
	groupNames     map[string]string // ID -> name for resolving remote group references
	cursor         int
	selectorScroll int

	// Detail state for currently selected SG
	servers     []serverRef
	ports       []network.Port
	serverNames map[string]string // DeviceID -> server name
	detailErr   string

	// Pane focus and cursors
	focus         focusPane
	ruleCursor    int
	rulesScroll   int
	serverCursor  int
	serversScroll int
	portsCursor   int
	portsScroll   int

	// UI state
	width           int
	height          int
	loading         bool
	detailLoading   bool
	spinner         spinner.Model
	err             string
	refreshInterval time.Duration
	highlightNames  map[string]bool
	lastDetailSGID  string // tracks which SG's detail is loaded
}

// New creates a security group view model.
func New(networkClient *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		networkClient:   networkClient,
		loading:         true,
		spinner:         s,
		refreshInterval: refreshInterval,
	}
}

// SetComputeClient sets the compute client for server resolution.
func (m *Model) SetComputeClient(client *gophercloud.ServiceClient) {
	m.computeClient = client
}

// Init starts the initial fetch.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[secgroupview] Init()")
	return tea.Batch(m.spinner.Tick, m.fetchGroups())
}

func (m Model) selectedSG() *network.SecurityGroup {
	if m.cursor >= 0 && m.cursor < len(m.groups) {
		return &m.groups[m.cursor]
	}
	return nil
}

// SelectedGroupID returns the ID of the currently selected group.
func (m Model) SelectedGroupID() string {
	if sg := m.selectedSG(); sg != nil {
		return sg.ID
	}
	return ""
}

// SelectedGroupName returns the name of the currently selected group.
func (m Model) SelectedGroupName() string {
	if sg := m.selectedSG(); sg != nil {
		return sg.Name
	}
	return ""
}

// CopyEntries returns the title and copyable fields for the selected
// security group, with extras for the focused rule or attached server
// when one of those panes has focus.
func (m Model) CopyEntries() (string, []copypicker.Entry) {
	sg := m.selectedSG()
	if sg == nil {
		return "", nil
	}
	b := copypicker.Builder{}
	b.Add("ID", sg.ID).Add("Name", sg.Name)
	if m.focus == FocusRules {
		if r := m.SelectedRule(); r != nil {
			b.Add("Rule ID", r.ID)
		}
	}
	if m.focus == focusServers {
		b.Add("Attached server ID", m.SelectedServerID())
	}
	return "Copy — security group " + sg.Name, b.Entries()
}

// SGDescription returns the description of the currently selected group.
func (m Model) SGDescription() string {
	if sg := m.selectedSG(); sg != nil {
		return sg.Description
	}
	return ""
}

// SelectedRuleID returns the ID of the currently selected rule, or "" if none.
func (m Model) SelectedRuleID() string {
	if m.focus != FocusRules {
		return ""
	}
	rules := m.sortedRules()
	if m.ruleCursor < 0 || m.ruleCursor >= len(rules) {
		return ""
	}
	return rules[m.ruleCursor].ID
}

// SelectedRule returns the currently selected rule, or nil if none.
func (m Model) SelectedRule() *network.SecurityRule {
	if m.focus != FocusRules {
		return nil
	}
	rules := m.sortedRules()
	if m.ruleCursor < 0 || m.ruleCursor >= len(rules) {
		return nil
	}
	r := rules[m.ruleCursor]
	return &r
}

// SelectedServerID returns the ID of the currently selected server, or "" if none.
func (m Model) SelectedServerID() string {
	if m.focus != focusServers {
		return ""
	}
	if m.serverCursor < 0 || m.serverCursor >= len(m.servers) {
		return ""
	}
	return m.servers[m.serverCursor].ID
}

// FocusedPane returns the currently focused pane.
func (m Model) FocusedPane() focusPane {
	return m.focus
}

// InRules returns true when the rules pane is focused (for app key handler compat).
func (m Model) InRules() bool {
	return m.focus == FocusRules
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sgLoadedMsg:
		shared.Debugf("[secgroupview] sgLoadedMsg: %d groups", len(msg.groups))
		var cursorID string
		if m.cursor >= 0 && m.cursor < len(m.groups) {
			cursorID = m.groups[m.cursor].ID
		}
		m.loading = false
		m.groups = msg.groups
		m.groupNames = make(map[string]string)
		for _, g := range msg.groups {
			m.groupNames[g.ID] = g.Name
		}
		// Restore cursor position
		if cursorID != "" {
			for i, g := range m.groups {
				if g.ID == cursorID {
					m.cursor = i
					break
				}
			}
		}
		if m.cursor >= len(m.groups) && len(m.groups) > 0 {
			m.cursor = len(m.groups) - 1
		}
		m.err = ""
		m.applyHighlightNames()
		// Fetch detail for selected SG if changed
		if sg := m.selectedSG(); sg != nil && sg.ID != m.lastDetailSGID {
			m.lastDetailSGID = sg.ID
			m.resetDetailState()
			return m, m.fetchDetail(sg.ID)
		}
		return m, nil

	case sgErrMsg:
		shared.Debugf("[secgroupview] sgErrMsg: %v", msg.err)
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case detailLoadedMsg:
		shared.Debugf("[secgroupview] detailLoadedMsg: %d servers, %d ports", len(msg.servers), len(msg.ports))
		// Only apply if this is still the selected SG
		if sg := m.selectedSG(); sg != nil && sg.ID == msg.sgID {
			m.detailLoading = false
			m.detailErr = ""
			m.servers = msg.servers
			m.ports = msg.ports
			m.serverNames = msg.serverNames
			m.clampDetailCursors()
		}
		return m, nil

	case detailErrMsg:
		shared.Debugf("[secgroupview] detailErrMsg: %v", msg.err)
		if sg := m.selectedSG(); sg != nil && sg.ID == msg.sgID {
			m.detailLoading = false
			m.detailErr = msg.err.Error()
		}
		return m, nil

	case shared.TickMsg:
		if m.loading {
			shared.Debugf("[secgroupview] tick skipped (loading)")
			return m, nil
		}
		shared.Debugf("[secgroupview] tick fetching")
		cmds := []tea.Cmd{m.fetchGroups()}
		if sg := m.selectedSG(); sg != nil {
			cmds = append(cmds, m.fetchDetail(sg.ID))
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		if m.loading || m.detailLoading {
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

func (m *Model) resetDetailState() {
	m.detailLoading = true
	m.detailErr = ""
	m.servers = nil
	m.ports = nil
	m.serverNames = nil
	m.ruleCursor = 0
	m.rulesScroll = 0
	m.serverCursor = 0
	m.serversScroll = 0
	m.portsCursor = 0
	m.portsScroll = 0
}

func (m *Model) clampDetailCursors() {
	if sg := m.selectedSG(); sg != nil {
		rules := m.sortedRules()
		if m.ruleCursor >= len(rules) {
			m.ruleCursor = max(0, len(rules)-1)
		}
	}
	if m.serverCursor >= len(m.servers) {
		m.serverCursor = max(0, len(m.servers)-1)
	}
	if m.portsCursor >= len(m.ports) {
		m.portsCursor = max(0, len(m.ports)-1)
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Tab):
		m.focus = (m.focus + 1) % focusPaneCount
		return m, nil

	case key.Matches(msg, shared.Keys.ShiftTab):
		m.focus = (m.focus + focusPaneCount - 1) % focusPaneCount
		return m, nil

	case key.Matches(msg, shared.Keys.Up):
		return m.scrollUp(1)

	case key.Matches(msg, shared.Keys.Down):
		return m.scrollDown(1)

	case key.Matches(msg, shared.Keys.PageUp):
		return m.scrollUp(10)

	case key.Matches(msg, shared.Keys.PageDown):
		return m.scrollDown(10)
	}
	return m, nil
}

func (m Model) scrollUp(n int) (Model, tea.Cmd) {
	switch m.focus {
	case FocusSelector:
		m.cursor -= n
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureSelectorCursorVisible()
		return m.onSelectorChange()
	case focusInfo:
		// no scrolling
	case FocusRules:
		m.ruleCursor -= n
		if m.ruleCursor < 0 {
			m.ruleCursor = 0
		}
		m.ensureRuleCursorVisible()
	case focusServers:
		m.serverCursor -= n
		if m.serverCursor < 0 {
			m.serverCursor = 0
		}
		m.ensureServerCursorVisible()
	case focusPorts:
		m.portsCursor -= n
		if m.portsCursor < 0 {
			m.portsCursor = 0
		}
		m.ensurePortCursorVisible()
	}
	return m, nil
}

func (m Model) scrollDown(n int) (Model, tea.Cmd) {
	switch m.focus {
	case FocusSelector:
		m.cursor += n
		maxIdx := len(m.groups) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.cursor > maxIdx {
			m.cursor = maxIdx
		}
		m.ensureSelectorCursorVisible()
		return m.onSelectorChange()
	case focusInfo:
		// no scrolling
	case FocusRules:
		rules := m.sortedRules()
		m.ruleCursor += n
		maxIdx := len(rules) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.ruleCursor > maxIdx {
			m.ruleCursor = maxIdx
		}
		m.ensureRuleCursorVisible()
	case focusServers:
		m.serverCursor += n
		maxIdx := len(m.servers) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.serverCursor > maxIdx {
			m.serverCursor = maxIdx
		}
		m.ensureServerCursorVisible()
	case focusPorts:
		m.portsCursor += n
		maxIdx := len(m.ports) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.portsCursor > maxIdx {
			m.portsCursor = maxIdx
		}
		m.ensurePortCursorVisible()
	}
	return m, nil
}

func (m Model) onSelectorChange() (Model, tea.Cmd) {
	sg := m.selectedSG()
	if sg == nil || sg.ID == m.lastDetailSGID {
		return m, nil
	}
	m.lastDetailSGID = sg.ID
	m.resetDetailState()
	return m, m.fetchDetail(sg.ID)
}

func (m *Model) ensureSelectorCursorVisible() {
	visH := m.selectorVisibleLines()
	if m.cursor < m.selectorScroll {
		m.selectorScroll = m.cursor
	}
	if m.cursor >= m.selectorScroll+visH {
		m.selectorScroll = m.cursor - visH + 1
	}
}

func (m *Model) ensureRuleCursorVisible() {
	visH := m.rulesVisibleLines()
	if m.ruleCursor < m.rulesScroll {
		m.rulesScroll = m.ruleCursor
	}
	if m.ruleCursor >= m.rulesScroll+visH {
		m.rulesScroll = m.ruleCursor - visH + 1
	}
}

func (m *Model) ensureServerCursorVisible() {
	visH := m.detailBottomVisibleLines()
	if m.serverCursor < m.serversScroll {
		m.serversScroll = m.serverCursor
	}
	if m.serverCursor >= m.serversScroll+visH {
		m.serversScroll = m.serverCursor - visH + 1
	}
}

func (m *Model) ensurePortCursorVisible() {
	visH := m.detailBottomVisibleLines()
	if m.portsCursor < m.portsScroll {
		m.portsScroll = m.portsCursor
	}
	if m.portsCursor >= m.portsScroll+visH {
		m.portsScroll = m.portsCursor - visH + 1
	}
}

// --- Height calculations ---

func (m Model) totalPanelHeight() int {
	h := m.height - 8 // title + blank + action bar + spacer + tab bar + status bar + newline separators
	if h < 10 {
		h = 10
	}
	return h
}

func (m Model) selectorHeight() int {
	h := len(m.groups) + 2 // content + border
	if h < 3 {
		h = 3
	}
	maxH := m.totalPanelHeight() * 30 / 100
	if maxH < 5 {
		maxH = 5
	}
	if h > maxH {
		h = maxH
	}
	return h
}

func (m Model) selectorVisibleLines() int {
	lines := m.selectorHeight() - 2 // border
	if lines < 1 {
		lines = 1
	}
	return lines
}

func (m Model) detailHeight() int {
	return m.totalPanelHeight() - m.selectorHeight()
}

func (m Model) rulesVisibleLines() int {
	dh := m.detailHeight()
	topH := dh * 55 / 100
	if topH < 6 {
		topH = 6
	}
	lines := topH - 5 // border(4) + header(1)
	if lines < 1 {
		lines = 1
	}
	return lines
}

func (m Model) detailBottomVisibleLines() int {
	dh := m.detailHeight()
	topH := dh * 55 / 100
	if topH < 6 {
		topH = 6
	}
	bottomH := dh - topH
	if bottomH < 4 {
		bottomH = 4
	}
	lines := bottomH - 5 // border(4) + header(1)
	if lines < 1 {
		lines = 1
	}
	return lines
}

// --- View ---

func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Security Groups")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.groups))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.groups) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No security groups found.") + "\n")
		return b.String()
	}

	if m.width < narrowThreshold {
		b.WriteString(m.renderNarrow())
	} else {
		b.WriteString(m.renderWide())
	}

	b.WriteString("\n" + m.renderActionBar() + "\n")

	return b.String()
}

func (m Model) renderWide() string {
	selH := m.selectorHeight()
	dh := m.detailHeight()

	// Selector: full width
	selContent := m.renderSelectorContent(m.width-4, selH-2)
	selPanel := m.panelBorder(FocusSelector).
		Width(m.width).
		Height(selH).
		Render(padContent(m.panelTitle(FocusSelector), selContent))

	// Detail top row: info (35%) | rules (65%)
	topH := dh * 55 / 100
	if topH < 6 {
		topH = 6
	}
	bottomH := dh - topH
	if bottomH < 4 {
		bottomH = 4
	}

	leftW := m.width * 35 / 100
	rightW := m.width - leftW - 1

	infoContent := padContent(m.panelTitle(focusInfo), m.renderInfoContent(leftW-4))
	infoPanel := m.panelBorder(focusInfo).Width(leftW).Height(topH).Render(infoContent)

	rulesContent := padContent(m.panelTitle(FocusRules), m.renderRulesContent(rightW-4, topH-4))
	rulesPanel := m.panelBorder(FocusRules).Width(rightW).Height(topH).Render(rulesContent)

	// Detail bottom row: servers (35%) | ports (65%)
	serversContent := padContent(m.panelTitle(focusServers), m.renderServersContent(leftW-4, bottomH-4))
	serversPanel := m.panelBorder(focusServers).Width(leftW).Height(bottomH).Render(serversContent)

	portsContent := padContent(m.panelTitle(focusPorts), m.renderPortsContent(rightW-4, bottomH-4))
	portsPanel := m.panelBorder(focusPorts).Width(rightW).Height(bottomH).Render(portsContent)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, infoPanel, " ", rulesPanel)
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, serversPanel, " ", portsPanel)

	return selPanel + "\n" + topRow + "\n" + bottomRow
}

func (m Model) renderNarrow() string {
	w := m.width - 2
	totalH := m.totalPanelHeight()

	selH := m.selectorHeight()
	remaining := totalH - selH
	infoH := remaining * 20 / 100
	rulesH := remaining * 35 / 100
	serversH := remaining * 22 / 100
	portsH := remaining - infoH - rulesH - serversH

	for _, h := range []*int{&infoH, &rulesH} {
		if *h < 4 {
			*h = 4
		}
	}
	for _, h := range []*int{&serversH, &portsH} {
		if *h < 3 {
			*h = 3
		}
	}

	selContent := m.renderSelectorContent(w-4, selH-2)
	selPanel := m.panelBorder(FocusSelector).Width(w).Height(selH).Render(padContent(m.panelTitle(FocusSelector), selContent))

	infoPanel := m.panelBorder(focusInfo).Width(w).Height(infoH).Render(padContent(m.panelTitle(focusInfo), m.renderInfoContent(w-4)))
	rulesPanel := m.panelBorder(FocusRules).Width(w).Height(rulesH).Render(padContent(m.panelTitle(FocusRules), m.renderRulesContent(w-4, rulesH-4)))
	serversPanel := m.panelBorder(focusServers).Width(w).Height(serversH).Render(padContent(m.panelTitle(focusServers), m.renderServersContent(w-4, serversH-4)))
	portsPanel := m.panelBorder(focusPorts).Width(w).Height(portsH).Render(padContent(m.panelTitle(focusPorts), m.renderPortsContent(w-4, portsH-4)))

	return lipgloss.JoinVertical(lipgloss.Left, selPanel, infoPanel, rulesPanel, serversPanel, portsPanel)
}

// --- Panel helpers ---

func padContent(title, content string) string {
	var out []string
	out = append(out, " "+title)
	out = append(out, "")
	if content != "" {
		for _, l := range strings.Split(content, "\n") {
			out = append(out, " "+l)
		}
	}
	return strings.Join(out, "\n")
}

func (m Model) panelTitle(pane focusPane) string {
	borderColor := shared.ColorMuted
	if m.focus == pane {
		borderColor = shared.ColorPrimary
	}
	titleStyle := lipgloss.NewStyle().Foreground(borderColor).Bold(true)

	switch pane {
	case FocusSelector:
		return titleStyle.Render("Security Groups")
	case focusInfo:
		return titleStyle.Render("Info")
	case FocusRules:
		t := titleStyle.Render("Rules")
		if m.detailLoading {
			t += " " + m.spinner.View()
		}
		return t
	case focusServers:
		t := titleStyle.Render("Servers")
		if m.detailLoading {
			t += " " + m.spinner.View()
		}
		return t
	case focusPorts:
		t := titleStyle.Render("Ports")
		if m.detailLoading {
			t += " " + m.spinner.View()
		}
		return t
	}
	return ""
}

func (m Model) panelBorder(pane focusPane) lipgloss.Style {
	borderColor := shared.ColorMuted
	if m.focus == pane {
		borderColor = shared.ColorPrimary
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		BorderTop(true).BorderBottom(true).BorderLeft(true).BorderRight(true)
}

// --- Selector rendering ---

func (m Model) renderSelectorContent(maxWidth, maxHeight int) string {
	if len(m.groups) == 0 {
		return ""
	}

	visibleLines := maxHeight
	if visibleLines < 1 {
		visibleLines = 1
	}

	var lines []string
	for i, sg := range m.groups {
		if i < m.selectorScroll {
			continue
		}
		if i >= m.selectorScroll+visibleLines {
			break
		}

		selected := i == m.cursor
		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		nameStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if selected {
			nameStyle = nameStyle.Foreground(shared.ColorHighlight).Bold(true)
		}

		rulesCount := fmt.Sprintf(" (%d rules)", len(sg.Rules))
		desc := ""
		if sg.Description != "" {
			desc = shared.StyleHelp.Render(" \u2014 " + sg.Description)
		}

		line := prefix + nameStyle.Render(sg.Name) + shared.StyleHelp.Render(rulesCount) + desc
		if lipgloss.Width(line) > maxWidth+2 {
			line = line[:maxWidth+1]
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// --- Info rendering ---

func (m Model) renderInfoContent(maxWidth int) string {
	sg := m.selectedSG()
	if sg == nil {
		return ""
	}

	labelW := 8
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(labelW)
	valueStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)

	type prop struct{ label, value string }

	var ingress, egress int
	for _, r := range sg.Rules {
		if r.Direction == "ingress" {
			ingress++
		} else {
			egress++
		}
	}
	rulesVal := fmt.Sprintf("%d", len(sg.Rules))
	if len(sg.Rules) > 0 {
		rulesVal += fmt.Sprintf(" (%d in, %d out)", ingress, egress)
	}

	allProps := []prop{
		{"Name", sg.Name},
		{"Desc", sg.Description},
		{"ID", sg.ID},
		{"Rules", rulesVal},
	}

	valW := maxWidth - labelW
	if valW < 4 {
		valW = 4
	}

	var rows []string
	for _, p := range allProps {
		if p.value == "" {
			continue
		}
		val := p.value
		if lipgloss.Width(val) > valW {
			val = val[:valW-1] + "\u2026"
		}
		rows = append(rows, labelStyle.Render(p.label)+valueStyle.Render(val))
	}
	return strings.Join(rows, "\n")
}

// --- Rules rendering ---

func (m Model) sortedRules() []network.SecurityRule {
	sg := m.selectedSG()
	if sg == nil {
		return nil
	}
	rules := make([]network.SecurityRule, len(sg.Rules))
	copy(rules, sg.Rules)
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Direction != rules[j].Direction {
			return rules[i].Direction == "ingress"
		}
		return rules[i].Protocol < rules[j].Protocol
	})
	return rules
}

func (m Model) formatRemote(r network.SecurityRule) string {
	remote := r.RemoteIPPrefix
	if remote == "" && r.RemoteGroupID != "" {
		if name, ok := m.groupNames[r.RemoteGroupID]; ok {
			remote = "group:" + name
		} else {
			remote = "group:" + r.RemoteGroupID[:min(8, len(r.RemoteGroupID))] + "\u2026"
		}
	}
	if remote == "" {
		remote = "any"
	}
	return remote
}

func isDefaultEgressRule(r network.SecurityRule) bool {
	return r.Direction == "egress" && r.Protocol == "" && r.RemoteIPPrefix == "" &&
		r.RemoteGroupID == "" && r.PortRangeMin == 0 && r.PortRangeMax == 0
}

func (m Model) renderRulesContent(maxWidth, maxHeight int) string {
	rules := m.sortedRules()
	if len(rules) == 0 {
		return shared.StyleHelp.Render("No rules \u2014 Ctrl+N to add")
	}

	const (
		dirW   = 9
		etherW = 4
		gap    = 2
	)

	protoW := len("Proto")
	portsW := len("Ports")
	for _, r := range rules {
		p := r.Protocol
		if p == "" {
			p = "any"
		}
		if len(p) > protoW {
			protoW = len(p)
		}
		var portStr string
		if r.PortRangeMin == 0 && r.PortRangeMax == 0 {
			portStr = "all"
		} else if r.PortRangeMin == r.PortRangeMax {
			portStr = fmt.Sprintf("%d", r.PortRangeMin)
		} else {
			portStr = fmt.Sprintf("%d-%d", r.PortRangeMin, r.PortRangeMax)
		}
		if len(portStr) > portsW {
			portsW = len(portStr)
		}
	}

	remoteW := len("Remote")
	for _, r := range rules {
		remote := m.formatRemote(r)
		if len(remote) > remoteW {
			remoteW = len(remote)
		}
	}
	maxRemote := maxWidth - 2 - dirW - protoW - portsW - etherW - gap*4
	if maxRemote < 8 {
		maxRemote = 8
	}
	if remoteW > maxRemote {
		remoteW = maxRemote
	}

	sep := strings.Repeat(" ", gap)
	headerStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	header := fmt.Sprintf("  %-*s%s%-*s%s%-*s%s%-*s%s%s",
		dirW, "Dir", sep, protoW, "Proto", sep, portsW, "Ports", sep, remoteW, "Remote", sep, "Eth")
	headerLine := headerStyle.Render(header)

	visibleLines := maxHeight - 1
	if visibleLines < 1 {
		visibleLines = 1
	}

	dirStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary)
	etherStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	mutedLine := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)

	var lines []string
	lines = append(lines, headerLine)

	for i, r := range rules {
		if i < m.rulesScroll {
			continue
		}
		if i >= m.rulesScroll+visibleLines {
			break
		}

		isDef := isDefaultEgressRule(r)
		selected := m.focus == FocusRules && i == m.ruleCursor

		proto := r.Protocol
		if proto == "" {
			proto = "any"
		}

		ports := ""
		if r.PortRangeMin == 0 && r.PortRangeMax == 0 {
			ports = "all"
		} else if r.PortRangeMin == r.PortRangeMax {
			ports = fmt.Sprintf("%d", r.PortRangeMin)
		} else {
			ports = fmt.Sprintf("%d-%d", r.PortRangeMin, r.PortRangeMax)
		}

		remote := m.formatRemote(r)
		if len(remote) > remoteW {
			remote = remote[:remoteW-1] + "\u2026"
		}

		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		dirCol := dirStyle.Render(fmt.Sprintf("%-*s", dirW, r.Direction))
		etherCol := etherStyle.Render(r.EtherType)

		line := fmt.Sprintf("%s%s%s%-*s%s%-*s%s%-*s%s%s",
			prefix, dirCol, sep, protoW, proto, sep, portsW, ports, sep, remoteW, remote, sep, etherCol)

		if selected {
			line = selectedBg.Render(line)
		} else if isDef {
			line = mutedLine.Render(line)
		}

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// --- Servers rendering ---

func (m Model) renderServersContent(maxWidth, maxHeight int) string {
	if m.detailErr != "" {
		return lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.detailErr)
	}
	if len(m.servers) == 0 && !m.detailLoading {
		return shared.StyleHelp.Render("No servers using this group")
	}

	nameW := len("Name")
	for _, srv := range m.servers {
		if len(srv.Name) > nameW {
			nameW = len(srv.Name)
		}
	}
	maxNameW := maxWidth - 2 - 2 - 12
	if maxNameW < 8 {
		maxNameW = 8
	}
	if nameW > maxNameW {
		nameW = maxNameW
	}

	headerStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	header := fmt.Sprintf("  %-*s  %s", nameW, "Name", "Status")
	headerLine := headerStyle.Render(header)

	visibleLines := maxHeight - 1
	if visibleLines < 1 {
		visibleLines = 1
	}

	selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)

	var lines []string
	lines = append(lines, headerLine)

	for i, srv := range m.servers {
		if i < m.serversScroll {
			continue
		}
		if i >= m.serversScroll+visibleLines {
			break
		}

		selected := m.focus == focusServers && i == m.serverCursor
		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		name := srv.Name
		if len(name) > nameW {
			name = name[:nameW-1] + "\u2026"
		}

		status := shared.StatusIcon(srv.Status) + srv.Status
		line := fmt.Sprintf("%s%-*s  %s", prefix, nameW, name, status)

		if selected {
			line = selectedBg.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// --- Ports rendering ---

func (m Model) renderPortsContent(maxWidth, maxHeight int) string {
	if m.detailErr != "" {
		return lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.detailErr)
	}
	if len(m.ports) == 0 && !m.detailLoading {
		return shared.StyleHelp.Render("No ports found")
	}

	const (
		macW = 17
		gap  = 2
	)

	serverW := len("Server")
	for _, p := range m.ports {
		name := m.serverNames[p.DeviceID]
		if len(name) > serverW {
			serverW = len(name)
		}
	}
	if serverW > 20 {
		serverW = 20
	}

	sep := strings.Repeat(" ", gap)
	ipsW := maxWidth - 2 - macW - serverW - gap*2
	if ipsW < 10 {
		ipsW = 10
	}

	headerStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	header := fmt.Sprintf("  %-*s%s%-*s%s%s", macW, "MAC", sep, serverW, "Server", sep, "IPs")
	headerLine := headerStyle.Render(header)

	visibleLines := maxHeight - 1
	if visibleLines < 1 {
		visibleLines = 1
	}

	selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)

	var lines []string
	lines = append(lines, headerLine)

	for i, p := range m.ports {
		if i < m.portsScroll {
			continue
		}
		if i >= m.portsScroll+visibleLines {
			break
		}

		selected := m.focus == focusPorts && i == m.portsCursor
		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		serverName := m.serverNames[p.DeviceID]
		if serverName == "" {
			serverName = "\u2014"
		}
		if len(serverName) > serverW {
			serverName = serverName[:serverW-1] + "\u2026"
		}

		var ips []string
		for _, ip := range p.FixedIPs {
			ips = append(ips, ip.IPAddress)
		}
		ipStr := strings.Join(ips, ", ")
		if len(ipStr) > ipsW {
			ipStr = ipStr[:ipsW-1] + "\u2026"
		}
		if len(ips) == 0 {
			ipStr = "\u2014"
		}

		line := fmt.Sprintf("%s%-*s%s%-*s%s%s",
			prefix, macW, p.MACAddress, sep, serverW, serverName, sep, ipStr)

		if selected {
			line = selectedBg.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// --- Action bar ---

type actionButton struct {
	key   string
	label string
}

func btn(k, label string) actionButton {
	return actionButton{key: k, label: label}
}

func (m Model) renderActionBar() string {
	sg := m.selectedSG()
	if sg == nil {
		return ""
	}

	isDefault := sg.Name == "default"
	var buttons []actionButton

	switch m.focus {
	case FocusSelector, focusInfo, focusPorts:
		buttons = append(buttons, btn("^n", "New Group"))
		if !isDefault {
			buttons = append(buttons, btn("^d", "Delete Group"))
			buttons = append(buttons, btn("r", "Rename"))
		}
		buttons = append(buttons, btn("c", "Clone"))
	case FocusRules:
		buttons = append(buttons, btn("^n", "Add Rule"))
		if m.SelectedRuleID() != "" {
			buttons = append(buttons, btn("^d", "Delete Rule"))
			buttons = append(buttons, btn("enter", "Edit Rule"))
		}
	case focusServers:
		if m.SelectedServerID() != "" {
			buttons = append(buttons, btn("enter", "Open Server"))
		}
	}

	keyStyle := lipgloss.NewStyle().
		Foreground(shared.ColorHighlight).
		Background(shared.ColorSecondary).
		Bold(true).Padding(0, 0)
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)

	var parts []string
	totalLen := 0
	maxWidth := m.width - 4

	for _, b := range buttons {
		part := keyStyle.Render("["+b.key+"]") + labelStyle.Render(b.label)
		partLen := len("["+b.key+"]") + len(b.label) + 1
		if totalLen+partLen > maxWidth && len(parts) > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("[?]More"))
			break
		}
		parts = append(parts, part)
		totalLen += partLen
	}

	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

// --- Public methods ---

// ForceRefresh triggers a manual reload.
func (m *Model) ForceRefresh() tea.Cmd {
	shared.Debugf("[secgroupview] ForceRefresh()")
	m.loading = true
	cmds := []tea.Cmd{m.spinner.Tick, m.fetchGroups()}
	if sg := m.selectedSG(); sg != nil {
		m.detailLoading = true
		cmds = append(cmds, m.fetchDetail(sg.ID))
	}
	return tea.Batch(cmds...)
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// ScrollToNames positions the cursor on the first matching group name.
func (m *Model) ScrollToNames(names []string) {
	m.highlightNames = make(map[string]bool, len(names))
	for _, n := range names {
		m.highlightNames[n] = true
	}
	m.applyHighlightNames()
}

func (m *Model) applyHighlightNames() {
	if len(m.highlightNames) == 0 {
		return
	}
	for i, sg := range m.groups {
		if m.highlightNames[sg.Name] {
			m.cursor = i
			m.ensureSelectorCursorVisible()
			m.highlightNames = nil
			// Trigger detail fetch
			if sg.ID != m.lastDetailSGID {
				m.lastDetailSGID = sg.ID
				m.resetDetailState()
			}
			return
		}
	}
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	switch m.focus {
	case FocusRules:
		return "↑↓ navigate • ^n add rule • ^d delete • enter edit • tab focus • R refresh • ? help"
	case focusServers:
		return "↑↓ navigate • enter open server • tab focus • R refresh • ? help"
	case focusPorts:
		return "↑↓ navigate • tab focus • R refresh • ? help"
	case focusInfo:
		return "tab focus • ^n new • r rename • c clone • R refresh • ? help"
	default:
		return "↑↓ navigate • ^n new • ^d delete • r rename • c clone • tab focus • R refresh • ? help"
	}
}

// --- Data fetching ---

func (m Model) fetchGroups() tea.Cmd {
	client := m.networkClient
	return func() tea.Msg {
		shared.Debugf("[secgroupview] fetchGroups start")
		groups, err := network.ListSecurityGroups(context.Background(), client)
		if err != nil {
			shared.Debugf("[secgroupview] fetchGroups error: %v", err)
			return sgErrMsg{err: err}
		}
		shared.Debugf("[secgroupview] fetchGroups done: %d groups", len(groups))
		return sgLoadedMsg{groups: groups}
	}
}

func (m Model) fetchDetail(sgID string) tea.Cmd {
	networkClient := m.networkClient
	computeClient := m.computeClient
	return func() tea.Msg {
		shared.Debugf("[secgroupview] fetchDetail start")
		if computeClient == nil {
			shared.Debugf("[secgroupview] fetchDetail done (no compute client)")
			return detailLoadedMsg{sgID: sgID}
		}

		fetchedPorts, err := network.ListPortsBySecurityGroup(context.Background(), networkClient, sgID)
		if err != nil {
			shared.Debugf("[secgroupview] fetchDetail error: %v", err)
			return detailErrMsg{sgID: sgID, err: err}
		}

		deviceIDs := make(map[string]bool)
		for _, p := range fetchedPorts {
			if strings.HasPrefix(p.DeviceOwner, "compute:") && p.DeviceID != "" {
				deviceIDs[p.DeviceID] = true
			}
		}

		if len(deviceIDs) == 0 {
			shared.Debugf("[secgroupview] fetchDetail done: 0 servers, %d ports", len(fetchedPorts))
			return detailLoadedMsg{sgID: sgID, ports: fetchedPorts}
		}

		allServers, err := compute.ListServers(context.Background(), computeClient)
		if err != nil {
			return detailErrMsg{sgID: sgID, err: err}
		}

		serverMap := make(map[string]compute.Server, len(allServers))
		for _, s := range allServers {
			serverMap[s.ID] = s
		}

		srvNames := make(map[string]string)
		var refs []serverRef
		for id := range deviceIDs {
			if s, ok := serverMap[id]; ok {
				refs = append(refs, serverRef{ID: s.ID, Name: s.Name, Status: s.Status})
				srvNames[s.ID] = s.Name
			}
		}

		sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
		sort.Slice(fetchedPorts, func(i, j int) bool {
			ni := srvNames[fetchedPorts[i].DeviceID]
			nj := srvNames[fetchedPorts[j].DeviceID]
			if ni != nj {
				return ni < nj
			}
			return fetchedPorts[i].MACAddress < fetchedPorts[j].MACAddress
		})

		shared.Debugf("[secgroupview] fetchDetail done: %d servers, %d ports", len(refs), len(fetchedPorts))
		return detailLoadedMsg{sgID: sgID, servers: refs, ports: fetchedPorts, serverNames: srvNames}
	}
}

