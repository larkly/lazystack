package lbdetail

import (
	"context"
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/shared"
)

// FocusPane identifies a pane in the load balancer detail view.
type FocusPane int

const (
	FocusInfo FocusPane = iota
	FocusListeners
	FocusPools
	FocusMembers
)

const FocusPaneCount = 4
const narrowThreshold = 80

var selectedRowStyle = lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)

type lbDetailLoadedMsg struct {
	lb        *loadbalancer.LoadBalancer
	listeners []loadbalancer.Listener
	pools     []loadbalancer.Pool
	members   map[string][]loadbalancer.Member
	monitors  map[string]*loadbalancer.HealthMonitor
}

type lbDetailErrMsg struct {
	err error
}

// Model is the load balancer detail view.
type Model struct {
	client    *gophercloud.ServiceClient
	lbID      string
	lb        *loadbalancer.LoadBalancer
	listeners []loadbalancer.Listener
	pools     []loadbalancer.Pool
	members   map[string][]loadbalancer.Member
	monitors  map[string]*loadbalancer.HealthMonitor
	loading   bool
	spinner   spinner.Model
	width     int
	height    int
	err       string

	// Pane focus and cursors
	focus          FocusPane
	listenerCursor int
	listenerScroll int
	poolCursor     int
	poolScroll     int
	memberCursor   int
	memberScroll   int

	// Bulk member selection
	selectedMembers map[string]bool
}

// New creates a load balancer detail model.
func New(client *gophercloud.ServiceClient, lbID string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:   client,
		lbID:     lbID,
		loading:  true,
		spinner:  s,
		members:         make(map[string][]loadbalancer.Member),
		monitors:         make(map[string]*loadbalancer.HealthMonitor),
		selectedMembers: make(map[string]bool),
	}
}

// Init fetches the load balancer details.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[lbdetail] Init()")
	return tea.Batch(m.spinner.Tick, m.fetchDetail())
}

// LBID returns the current load balancer ID.
func (m Model) LBID() string {
	return m.lbID
}

// LBName returns the current load balancer name.
func (m Model) LBName() string {
	if m.lb != nil {
		if m.lb.Name != "" {
			return m.lb.Name
		}
		return m.lbID
	}
	return m.lbID
}

// LB returns the current load balancer, or nil if not loaded.
func (m Model) LB() *loadbalancer.LoadBalancer {
	return m.lb
}

// FocusedPane returns the currently focused pane.
func (m Model) FocusedPane() FocusPane {
	return m.focus
}

// SelectedListenerID returns the ID of the currently selected listener, or "".
func (m Model) SelectedListenerID() string {
	if m.listenerCursor >= 0 && m.listenerCursor < len(m.listeners) {
		return m.listeners[m.listenerCursor].ID
	}
	return ""
}

// SelectedListenerName returns the name of the currently selected listener.
func (m Model) SelectedListenerName() string {
	if m.listenerCursor >= 0 && m.listenerCursor < len(m.listeners) {
		l := m.listeners[m.listenerCursor]
		if l.Name != "" {
			return l.Name
		}
		return fmt.Sprintf("%s:%d", l.Protocol, l.ProtocolPort)
	}
	return ""
}

// SelectedPoolID returns the ID of the currently selected pool, or "".
func (m Model) SelectedPoolID() string {
	if m.poolCursor >= 0 && m.poolCursor < len(m.pools) {
		return m.pools[m.poolCursor].ID
	}
	return ""
}

// SelectedPoolName returns the name of the currently selected pool.
func (m Model) SelectedPoolName() string {
	if m.poolCursor >= 0 && m.poolCursor < len(m.pools) {
		return m.pools[m.poolCursor].Name
	}
	return ""
}

// SelectedMemberID returns the ID of the currently selected member, or "".
func (m Model) SelectedMemberID() string {
	members := m.selectedPoolMembers()
	if m.memberCursor >= 0 && m.memberCursor < len(members) {
		return members[m.memberCursor].ID
	}
	return ""
}

// SelectedMemberName returns a display name for the currently selected member.
func (m Model) SelectedMemberName() string {
	members := m.selectedPoolMembers()
	if m.memberCursor >= 0 && m.memberCursor < len(members) {
		mem := members[m.memberCursor]
		if mem.Name != "" {
			return mem.Name
		}
		return fmt.Sprintf("%s:%d", mem.Address, mem.ProtocolPort)
	}
	return ""
}

// SelectedListener returns the full Listener struct for the cursor, or nil.
func (m Model) SelectedListener() *loadbalancer.Listener {
	if m.listenerCursor >= 0 && m.listenerCursor < len(m.listeners) {
		l := m.listeners[m.listenerCursor]
		return &l
	}
	return nil
}

// SelectedPool returns the full Pool struct for the cursor, or nil.
func (m Model) SelectedPool() *loadbalancer.Pool {
	if m.poolCursor >= 0 && m.poolCursor < len(m.pools) {
		p := m.pools[m.poolCursor]
		return &p
	}
	return nil
}

// SelectedMember returns the full Member struct for the cursor, or nil.
func (m Model) SelectedMember() *loadbalancer.Member {
	members := m.selectedPoolMembers()
	if m.memberCursor >= 0 && m.memberCursor < len(members) {
		mem := members[m.memberCursor]
		return &mem
	}
	return nil
}

// Listeners returns the current listeners list.
func (m Model) Listeners() []loadbalancer.Listener {
	return m.listeners
}

// Pools returns the current pools list.
func (m Model) Pools() []loadbalancer.Pool {
	return m.pools
}

// TotalMemberCount returns the total number of members across all pools.
func (m Model) TotalMemberCount() int {
	total := 0
	for _, mems := range m.members {
		total += len(mems)
	}
	return total
}

// SelectedPoolMonitor returns the health monitor for the selected pool, or nil.
func (m Model) SelectedPoolMonitor() *loadbalancer.HealthMonitor {
	return m.selectedPoolMonitor()
}

// SelectedPoolMembers returns the members of the currently selected pool.
func (m Model) SelectedPoolMembers() []loadbalancer.Member {
	members := m.selectedPoolMembers()
	if len(members) == 0 {
		return nil
	}
	out := make([]loadbalancer.Member, len(members))
	copy(out, members)
	return out
}

// ToggleMemberSelection toggles the selection state of the current member.
func (m *Model) ToggleMemberSelection() {
	mem := m.SelectedMember()
	if mem == nil {
		return
	}
	if m.selectedMembers[mem.ID] {
		delete(m.selectedMembers, mem.ID)
	} else {
		m.selectedMembers[mem.ID] = true
	}
}

// ToggleAllMemberSelection selects all or deselects all members in current pool.
func (m *Model) ToggleAllMemberSelection() {
	members := m.selectedPoolMembers()
	if len(members) == 0 {
		return
	}
	// If all are selected, deselect all; otherwise select all
	allSelected := true
	for _, mem := range members {
		if !m.selectedMembers[mem.ID] {
			allSelected = false
			break
		}
	}
	if allSelected {
		for _, mem := range members {
			delete(m.selectedMembers, mem.ID)
		}
	} else {
		for _, mem := range members {
			m.selectedMembers[mem.ID] = true
		}
	}
}

// SelectedMemberIDs returns the IDs of selected members in the current pool.
func (m Model) SelectedMemberIDs() []string {
	members := m.selectedPoolMembers()
	var ids []string
	for _, mem := range members {
		if m.selectedMembers[mem.ID] {
			ids = append(ids, mem.ID)
		}
	}
	return ids
}

// SelectedMemberCount returns the number of selected members in the current pool.
func (m Model) SelectedMemberCount() int {
	return len(m.SelectedMemberIDs())
}

// ClearMemberSelection clears all member selections.
func (m *Model) ClearMemberSelection() {
	m.selectedMembers = make(map[string]bool)
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case lbDetailLoadedMsg:
		shared.Debugf("[lbdetail] lbDetailLoadedMsg: %d listeners, %d pools", len(msg.listeners), len(msg.pools))
		m.loading = false
		m.lb = msg.lb
		m.listeners = msg.listeners
		m.pools = msg.pools
		m.members = msg.members
		m.monitors = msg.monitors
		m.err = ""
		m.clampCursors()
		return m, nil

	case lbDetailErrMsg:
		shared.Debugf("[lbdetail] lbDetailErrMsg: %v", msg.err)
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case shared.TickMsg:
		if m.loading {
			shared.Debugf("[lbdetail] tick skipped (loading)")
			return m, nil
		}
		shared.Debugf("[lbdetail] tick fetching")
		return m, m.fetchDetail()

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
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		return m, func() tea.Msg {
			return shared.ViewChangeMsg{View: "lblist"}
		}

	case key.Matches(msg, shared.Keys.Tab):
		m.focus = (m.focus + 1) % FocusPaneCount
		return m, nil

	case key.Matches(msg, shared.Keys.ShiftTab):
		m.focus = (m.focus + FocusPaneCount - 1) % FocusPaneCount
		return m, nil

	case key.Matches(msg, shared.Keys.Up):
		return m.scrollUp(1), nil

	case key.Matches(msg, shared.Keys.Down):
		return m.scrollDown(1), nil

	case key.Matches(msg, shared.Keys.PageUp):
		return m.scrollUp(10), nil

	case key.Matches(msg, shared.Keys.PageDown):
		return m.scrollDown(10), nil
	}
	return m, nil
}

func (m Model) scrollUp(n int) Model {
	switch m.focus {
	case FocusListeners:
		m.listenerCursor -= n
		if m.listenerCursor < 0 {
			m.listenerCursor = 0
		}
		m.ensureListenerCursorVisible()
	case FocusPools:
		prev := m.poolCursor
		m.poolCursor -= n
		if m.poolCursor < 0 {
			m.poolCursor = 0
		}
		m.ensurePoolCursorVisible()
		if m.poolCursor != prev {
			m.memberCursor = 0
			m.memberScroll = 0
			m.selectedMembers = make(map[string]bool)
		}
	case FocusMembers:
		m.memberCursor -= n
		if m.memberCursor < 0 {
			m.memberCursor = 0
		}
		m.ensureMemberCursorVisible()
	}
	return m
}

func (m Model) scrollDown(n int) Model {
	switch m.focus {
	case FocusListeners:
		m.listenerCursor += n
		maxIdx := len(m.listeners) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.listenerCursor > maxIdx {
			m.listenerCursor = maxIdx
		}
		m.ensureListenerCursorVisible()
	case FocusPools:
		prev := m.poolCursor
		m.poolCursor += n
		maxIdx := len(m.pools) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.poolCursor > maxIdx {
			m.poolCursor = maxIdx
		}
		m.ensurePoolCursorVisible()
		if m.poolCursor != prev {
			m.memberCursor = 0
			m.memberScroll = 0
			m.selectedMembers = make(map[string]bool)
		}
	case FocusMembers:
		m.memberCursor += n
		members := m.selectedPoolMembers()
		maxIdx := len(members) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.memberCursor > maxIdx {
			m.memberCursor = maxIdx
		}
		m.ensureMemberCursorVisible()
	}
	return m
}

func (m *Model) clampCursors() {
	if m.listenerCursor >= len(m.listeners) {
		m.listenerCursor = max(0, len(m.listeners)-1)
	}
	if m.poolCursor >= len(m.pools) {
		m.poolCursor = max(0, len(m.pools)-1)
	}
	members := m.selectedPoolMembers()
	if m.memberCursor >= len(members) {
		m.memberCursor = max(0, len(members)-1)
	}
}

func (m Model) selectedPoolID() string {
	if m.poolCursor >= 0 && m.poolCursor < len(m.pools) {
		return m.pools[m.poolCursor].ID
	}
	return ""
}

func (m Model) selectedPoolMembers() []loadbalancer.Member {
	id := m.selectedPoolID()
	if id == "" {
		return nil
	}
	return m.members[id]
}

func (m Model) selectedPoolMonitor() *loadbalancer.HealthMonitor {
	if m.poolCursor >= 0 && m.poolCursor < len(m.pools) {
		return m.monitors[m.pools[m.poolCursor].MonitorID]
	}
	return nil
}

// --- Cursor visibility ---

func (m *Model) ensureListenerCursorVisible() {
	visH := m.topVisibleLines()
	if m.listenerCursor < m.listenerScroll {
		m.listenerScroll = m.listenerCursor
	}
	if m.listenerCursor >= m.listenerScroll+visH {
		m.listenerScroll = m.listenerCursor - visH + 1
	}
}

func (m *Model) ensurePoolCursorVisible() {
	visH := m.bottomVisibleLines()
	// Account for health monitor reserved space
	if m.selectedPoolMonitor() != nil {
		visH -= 8
		if visH < 1 {
			visH = 1
		}
	}
	if m.poolCursor < m.poolScroll {
		m.poolScroll = m.poolCursor
	}
	if m.poolCursor >= m.poolScroll+visH {
		m.poolScroll = m.poolCursor - visH + 1
	}
}

func (m *Model) ensureMemberCursorVisible() {
	visH := m.bottomVisibleLines()
	if m.memberCursor < m.memberScroll {
		m.memberScroll = m.memberCursor
	}
	if m.memberCursor >= m.memberScroll+visH {
		m.memberScroll = m.memberCursor - visH + 1
	}
}

// --- Height calculations ---

func (m Model) totalPanelHeight() int {
	h := m.height - 6 // title + blank + action bar + spacer + status bar + newline
	if h < 10 {
		h = 10
	}
	return h
}

func (m Model) topHeight() int {
	h := m.totalPanelHeight() * 45 / 100
	if h < 6 {
		h = 6
	}
	return h
}

func (m Model) bottomHeight() int {
	h := m.totalPanelHeight() - m.topHeight()
	if h < 6 {
		h = 6
	}
	return h
}

func (m Model) topVisibleLines() int {
	lines := m.topHeight() - 5 // border(4) + header(1)
	if lines < 1 {
		lines = 1
	}
	return lines
}

func (m Model) bottomVisibleLines() int {
	lines := m.bottomHeight() - 5 // border(4) + header(1)
	if lines < 1 {
		lines = 1
	}
	return lines
}

// --- View ---

func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Load Balancer Detail")
	if m.loading {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if m.lb == nil {
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
	topH := m.topHeight()
	bottomH := m.bottomHeight()

	leftW := m.width * 35 / 100
	rightW := m.width - leftW - 1

	infoContent := padContent(m.panelTitle(FocusInfo), m.renderInfoContent(leftW-4))
	infoPanel := m.panelBorder(FocusInfo).Width(leftW).Height(topH).Render(infoContent)

	listenersContent := padContent(m.panelTitle(FocusListeners), m.renderListenersContent(rightW-4, topH-4))
	listenersPanel := m.panelBorder(FocusListeners).Width(rightW).Height(topH).Render(listenersContent)

	poolsContent := padContent(m.panelTitle(FocusPools), m.renderPoolsContent(leftW-4, bottomH-4))
	poolsPanel := m.panelBorder(FocusPools).Width(leftW).Height(bottomH).Render(poolsContent)

	membersContent := padContent(m.panelTitle(FocusMembers), m.renderMembersContent(rightW-4, bottomH-4))
	membersPanel := m.panelBorder(FocusMembers).Width(rightW).Height(bottomH).Render(membersContent)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, infoPanel, " ", listenersPanel)
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, poolsPanel, " ", membersPanel)

	return topRow + "\n" + bottomRow
}

func (m Model) renderNarrow() string {
	w := m.width - 2
	totalH := m.totalPanelHeight()

	infoH := totalH * 25 / 100
	listenersH := totalH * 25 / 100
	poolsH := totalH * 25 / 100
	membersH := totalH - infoH - listenersH - poolsH

	for _, h := range []*int{&infoH, &listenersH, &poolsH, &membersH} {
		if *h < 4 {
			*h = 4
		}
	}

	infoPanel := m.panelBorder(FocusInfo).Width(w).Height(infoH).Render(padContent(m.panelTitle(FocusInfo), m.renderInfoContent(w-4)))
	listenersPanel := m.panelBorder(FocusListeners).Width(w).Height(listenersH).Render(padContent(m.panelTitle(FocusListeners), m.renderListenersContent(w-4, listenersH-4)))
	poolsPanel := m.panelBorder(FocusPools).Width(w).Height(poolsH).Render(padContent(m.panelTitle(FocusPools), m.renderPoolsContent(w-4, poolsH-4)))
	membersPanel := m.panelBorder(FocusMembers).Width(w).Height(membersH).Render(padContent(m.panelTitle(FocusMembers), m.renderMembersContent(w-4, membersH-4)))

	return lipgloss.JoinVertical(lipgloss.Left, infoPanel, listenersPanel, poolsPanel, membersPanel)
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

func (m Model) panelTitle(pane FocusPane) string {
	borderColor := shared.ColorMuted
	if m.focus == pane {
		borderColor = shared.ColorPrimary
	}
	titleStyle := lipgloss.NewStyle().Foreground(borderColor).Bold(true)

	switch pane {
	case FocusInfo:
		return titleStyle.Render("Info")
	case FocusListeners:
		t := titleStyle.Render("Listeners")
		if m.loading {
			t += " " + m.spinner.View()
		}
		return t
	case FocusPools:
		t := titleStyle.Render("Pools")
		if m.loading {
			t += " " + m.spinner.View()
		}
		return t
	case FocusMembers:
		t := titleStyle.Render("Members")
		poolName := ""
		if m.poolCursor >= 0 && m.poolCursor < len(m.pools) {
			poolName = m.pools[m.poolCursor].Name
		}
		if poolName != "" {
			t += " " + lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("("+poolName+")")
		}
		if m.loading {
			t += " " + m.spinner.View()
		}
		return t
	}
	return ""
}

func (m Model) panelBorder(pane FocusPane) lipgloss.Style {
	borderColor := shared.ColorMuted
	if m.focus == pane {
		borderColor = shared.ColorPrimary
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		BorderTop(true).BorderBottom(true).BorderLeft(true).BorderRight(true)
}

// --- Info rendering ---

func (m Model) renderInfoContent(maxWidth int) string {
	if m.lb == nil {
		return ""
	}

	lb := m.lb
	labelW := 12
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(labelW)
	valueStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)

	type prop struct {
		label string
		value string
		style func(string) lipgloss.Style
	}

	allProps := []prop{
		{"Name", lb.Name, nil},
		{"ID", lb.ID, nil},
		{"VIP Address", lb.VipAddress, nil},
		{"Prov Status", lb.ProvisioningStatus, provStatusStyleFn},
		{"Oper Status", lb.OperatingStatus, operStatusStyleFn},
		{"Provider", lb.Provider, nil},
		{"Description", lb.Description, nil},
	}
	if !lb.AdminStateUp {
		allProps = append(allProps, prop{"Admin", "DOWN", func(s string) lipgloss.Style {
			return lipgloss.NewStyle().Foreground(shared.ColorError)
		}})
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
		label := labelStyle.Render(p.label)
		val := p.value
		if lipgloss.Width(val) > valW {
			val = val[:valW-1] + "\u2026"
		}
		var value string
		if p.style != nil {
			value = p.style(p.value).Render(shared.StatusIcon(p.value) + val)
		} else {
			value = valueStyle.Render(val)
		}
		rows = append(rows, label+value)
	}

	// Summary line
	rows = append(rows, "")
	summaryStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	rows = append(rows, summaryStyle.Render(fmt.Sprintf("%d listeners, %d pools", len(m.listeners), len(m.pools))))

	return strings.Join(rows, "\n")
}

// --- Listeners rendering ---

func (m Model) renderListenersContent(maxWidth, maxHeight int) string {
	if len(m.listeners) == 0 {
		return shared.StyleHelp.Render("No listeners configured")
	}

	// Build pool name lookup
	poolNames := make(map[string]string, len(m.pools))
	for _, p := range m.pools {
		poolNames[p.ID] = p.Name
	}

	const gap = 2
	sep := strings.Repeat(" ", gap)

	nameW := len("Name")
	for _, l := range m.listeners {
		n := l.Name
		if n == "" {
			n = l.Protocol
		}
		if len(n) > nameW {
			nameW = len(n)
		}
	}
	if nameW > 20 {
		nameW = 20
	}

	protoW := len("Protocol")
	portW := len("Port")
	for _, l := range m.listeners {
		ps := fmt.Sprintf("%d", l.ProtocolPort)
		if len(ps) > portW {
			portW = len(ps)
		}
		if len(l.Protocol) > protoW {
			protoW = len(l.Protocol)
		}
	}

	poolW := maxWidth - nameW - protoW - portW - gap*3 - 2
	if poolW < 8 {
		poolW = 8
	}

	headerStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	header := fmt.Sprintf("  %-*s%s%-*s%s%-*s%s%s",
		nameW, "Name", sep, protoW, "Protocol", sep, portW, "Port", sep, "Default Pool")
	headerLine := headerStyle.Render(header)

	visibleLines := maxHeight - 1
	if visibleLines < 1 {
		visibleLines = 1
	}

	var lines []string
	lines = append(lines, headerLine)

	for i, l := range m.listeners {
		if i < m.listenerScroll {
			continue
		}
		if i >= m.listenerScroll+visibleLines {
			break
		}

		selected := m.focus == FocusListeners && i == m.listenerCursor
		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		name := l.Name
		if name == "" {
			name = l.Protocol
		}
		if len(name) > nameW {
			name = name[:nameW-1] + "\u2026"
		}

		pool := poolNames[l.DefaultPoolID]
		if pool == "" && l.DefaultPoolID != "" {
			pool = l.DefaultPoolID[:min(8, len(l.DefaultPoolID))] + "\u2026"
		}
		if pool == "" {
			pool = "\u2014"
		}
		if len(pool) > poolW {
			pool = pool[:poolW-1] + "\u2026"
		}

		line := fmt.Sprintf("%s%-*s%s%-*s%s%-*d%s%s",
			prefix, nameW, name, sep, protoW, l.Protocol, sep, portW, l.ProtocolPort, sep, pool)

		if selected {
			line = selectedRowStyle.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// --- Pools rendering ---

func (m Model) renderPoolsContent(maxWidth, maxHeight int) string {
	if len(m.pools) == 0 {
		return shared.StyleHelp.Render("No pools configured")
	}

	nameW := len("Pool")
	for _, p := range m.pools {
		if len(p.Name) > nameW {
			nameW = len(p.Name)
		}
	}
	maxNameW := maxWidth - 2 - 14 // room for method + health indicator
	if maxNameW < 8 {
		maxNameW = 8
	}
	if nameW > maxNameW {
		nameW = maxNameW
	}

	methodW := len("Method")
	for _, p := range m.pools {
		if len(p.LBMethod) > methodW {
			methodW = len(p.LBMethod)
		}
	}
	if methodW > 16 {
		methodW = 16
	}

	headerStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	header := fmt.Sprintf("  %-*s  %-*s  %s", nameW, "Pool", methodW, "Method", "Hlth")
	headerLine := headerStyle.Render(header)

	// Reserve lines for health monitor details when a monitor exists
	monReserve := 0
	if m.selectedPoolMonitor() != nil {
		monReserve = 8 // title + blank + up to 6 detail lines
	}

	poolVisibleLines := maxHeight - 1 - monReserve
	if poolVisibleLines < 1 {
		poolVisibleLines = 1
	}

	var lines []string
	lines = append(lines, headerLine)

	for i, p := range m.pools {
		if i < m.poolScroll {
			continue
		}
		if i >= m.poolScroll+poolVisibleLines {
			break
		}

		selected := m.focus == FocusPools && i == m.poolCursor
		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		name := p.Name
		if len(name) > nameW {
			name = name[:nameW-1] + "\u2026"
		}

		method := p.LBMethod
		if len(method) > methodW {
			method = method[:methodW-1] + "\u2026"
		}

		// Health monitor indicator
		health := "\u2014"
		if mon := m.monitors[p.MonitorID]; mon != nil {
			health = mon.Type
			if mon.Type == "HTTP" || mon.Type == "HTTPS" {
				health = mon.Type + " " + mon.URLPath
			}
			// Truncate if too long
			maxHW := maxWidth - nameW - methodW - 8
			if maxHW < 4 {
				maxHW = 4
			}
			if len(health) > maxHW {
				health = health[:maxHW-1] + "\u2026"
			}
		}

		memberCount := len(m.members[p.ID])
		countStr := lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(fmt.Sprintf(" [%d]", memberCount))

		line := fmt.Sprintf("%s%-*s  %-*s  %s",
			prefix, nameW, name, methodW, method, health)

		if selected {
			line = selectedRowStyle.Render(line)
		}
		line += countStr
		lines = append(lines, line)
	}

	// Show health monitor details for the selected pool
	if mon := m.selectedPoolMonitor(); mon != nil {
		lines = append(lines, "")
		monStyle := lipgloss.NewStyle().Foreground(shared.ColorCyan)
		labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary)
		lines = append(lines, monStyle.Render("  \u2665 Health Monitor"))

		details := []struct{ k, v string }{
			{"Type", mon.Type},
			{"Interval", fmt.Sprintf("%ds delay, %ds timeout", mon.Delay, mon.Timeout)},
			{"Retries", fmt.Sprintf("%d up / %d down", mon.MaxRetries, mon.MaxRetriesDown)},
		}
		if mon.URLPath != "" {
			details = append(details, struct{ k, v string }{"Path", mon.HTTPMethod + " " + mon.URLPath})
		}
		if mon.ExpectedCodes != "" {
			details = append(details, struct{ k, v string }{"Expect", mon.ExpectedCodes})
		}
		if mon.OperatingStatus != "" {
			details = append(details, struct{ k, v string }{"Status", shared.StatusIcon(mon.OperatingStatus) + mon.OperatingStatus})
		}

		remaining := monReserve - 2 // title + blank already added
		for _, d := range details {
			if remaining <= 0 {
				break
			}
			lines = append(lines, fmt.Sprintf("    %s %s", labelStyle.Width(9).Render(d.k), d.v))
			remaining--
		}
	}

	return strings.Join(lines, "\n")
}

// --- Members rendering ---

func (m Model) renderMembersContent(maxWidth, maxHeight int) string {
	members := m.selectedPoolMembers()
	if len(m.pools) == 0 {
		return shared.StyleHelp.Render("No pools to show members for")
	}
	if len(members) == 0 {
		return shared.StyleHelp.Render("No members in this pool")
	}

	const gap = 2
	sep := strings.Repeat(" ", gap)

	// Calculate column widths
	addrW := len("Address")
	for _, mem := range members {
		addr := fmt.Sprintf("%s:%d", mem.Address, mem.ProtocolPort)
		if len(addr) > addrW {
			addrW = len(addr)
		}
	}
	if addrW > 24 {
		addrW = 24
	}

	nameW := len("Name")
	for _, mem := range members {
		if len(mem.Name) > nameW {
			nameW = len(mem.Name)
		}
	}
	maxNameW := maxWidth - addrW - 6 - 12 - gap*3 - 2 // weight(6) + status(12)
	if maxNameW < 6 {
		maxNameW = 6
	}
	if nameW > maxNameW {
		nameW = maxNameW
	}

	headerStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	header := fmt.Sprintf("  %-*s%s%-*s%s%-6s%s%s",
		nameW, "Name", sep, addrW, "Address", sep, "Weight", sep, "Status")
	headerLine := headerStyle.Render(header)

	visibleLines := maxHeight - 1
	if visibleLines < 1 {
		visibleLines = 1
	}

	var lines []string
	lines = append(lines, headerLine)

	for i, mem := range members {
		if i < m.memberScroll {
			continue
		}
		if i >= m.memberScroll+visibleLines {
			break
		}

		selected := m.focus == FocusMembers && i == m.memberCursor
		isChecked := m.selectedMembers[mem.ID]
		var prefix string
		if isChecked && selected {
			prefix = "\u25b8\u2713"
		} else if isChecked {
			prefix = " \u2713"
		} else if selected {
			prefix = "\u25b8 "
		} else {
			prefix = "  "
		}

		name := mem.Name
		if name == "" {
			name = "\u2014"
		}
		if len(name) > nameW {
			name = name[:nameW-1] + "\u2026"
		}

		addr := fmt.Sprintf("%s:%d", mem.Address, mem.ProtocolPort)
		if len(addr) > addrW {
			addr = addr[:addrW-1] + "\u2026"
		}

		var weight string
		if mem.Weight == 0 {
			weight = lipgloss.NewStyle().Foreground(shared.ColorWarning).Render("0 drn")
		} else {
			weight = fmt.Sprintf("%d", mem.Weight)
		}
		displayStatus := mem.OperatingStatus
		if !mem.AdminStateUp {
			displayStatus = "DISABLED"
		}
		status := shared.StatusIcon(displayStatus) + displayStatus
		statusStyle := memberStatusStyle(displayStatus)

		line := fmt.Sprintf("%s%-*s%s%-*s%s%-6s%s%s",
			prefix, nameW, name, sep, addrW, addr, sep, weight, sep,
			statusStyle.Render(status))

		if selected {
			line = selectedRowStyle.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// --- Action bar ---

func (m Model) renderActionBar() string {
	if m.lb == nil {
		return ""
	}

	// Show PENDING status instead of actions when LB is provisioning
	if strings.HasPrefix(m.lb.ProvisioningStatus, "PENDING_") {
		return " " + lipgloss.NewStyle().Foreground(shared.ColorWarning).Render(
			"\u25b2 "+m.lb.ProvisioningStatus+" \u2014 operations disabled")
	}

	keyStyle := lipgloss.NewStyle().
		Foreground(shared.ColorHighlight).
		Background(shared.ColorSecondary).
		Bold(true).Padding(0, 0)
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)

	type btn struct{ key, label string }
	var buttons []btn

	switch m.focus {
	case FocusInfo:
		buttons = append(buttons, btn{"enter", "Edit LB"})
		if m.lb.AdminStateUp {
			buttons = append(buttons, btn{"o", "Disable"})
		} else {
			buttons = append(buttons, btn{"o", "Enable"})
		}
		buttons = append(buttons, btn{"^d", "Delete LB"})
	case FocusListeners:
		buttons = append(buttons, btn{"^n", "Add Listener"})
		if l := m.SelectedListener(); l != nil {
			buttons = append(buttons, btn{"enter", "Edit"})
			if l.AdminStateUp {
				buttons = append(buttons, btn{"o", "Disable"})
			} else {
				buttons = append(buttons, btn{"o", "Enable"})
			}
			buttons = append(buttons, btn{"^d", "Delete Listener"})
		}
	case FocusPools:
		buttons = append(buttons, btn{"^n", "Add Pool"})
		if pool := m.SelectedPool(); pool != nil {
			buttons = append(buttons, btn{"enter", "Edit Pool"})
			if pool.MonitorID != "" {
				buttons = append(buttons, btn{"^h", "Edit Monitor"})
			} else {
				buttons = append(buttons, btn{"^h", "Add Monitor"})
			}
			if pool.AdminStateUp {
				buttons = append(buttons, btn{"o", "Disable"})
			} else {
				buttons = append(buttons, btn{"o", "Enable"})
			}
			if pool.MonitorID != "" {
				buttons = append(buttons, btn{"^d", "Delete Monitor"})
			} else {
				buttons = append(buttons, btn{"^d", "Delete Pool"})
			}
		}
	case FocusMembers:
		selCount := m.SelectedMemberCount()
		if selCount > 0 {
			buttons = append(buttons, btn{"space", "Toggle"})
			buttons = append(buttons, btn{"x", "All"})
			buttons = append(buttons, btn{"^d", fmt.Sprintf("Delete %d", selCount)})
		} else {
			if m.SelectedPoolID() != "" {
				buttons = append(buttons, btn{"^n", "Add Member"})
			}
			if mem := m.SelectedMember(); mem != nil {
				buttons = append(buttons, btn{"enter", "Edit"})
				if mem.Weight > 0 {
					buttons = append(buttons, btn{"w", "Drain"})
				}
				buttons = append(buttons, btn{"space", "Select"})
				if mem.AdminStateUp {
					buttons = append(buttons, btn{"o", "Disable"})
				} else {
					buttons = append(buttons, btn{"o", "Enable"})
				}
				buttons = append(buttons, btn{"^d", "Delete Member"})
			}
		}
	}
	buttons = append(buttons, btn{"tab", "Switch Pane"}, btn{"esc", "Back"})

	var parts []string
	totalLen := 0
	maxWidth := m.width - 4

	for _, b := range buttons {
		part := keyStyle.Render("["+b.key+"]") + labelStyle.Render(b.label)
		partLen := len("["+b.key+"]") + len(b.label) + 1
		if totalLen+partLen > maxWidth && len(parts) > 0 {
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

// --- Style helpers ---

func provStatusStyleFn(status string) lipgloss.Style {
	var fg color.Color = shared.ColorFg
	switch {
	case status == "ACTIVE":
		fg = shared.ColorSuccess
	case strings.HasPrefix(status, "PENDING_"):
		fg = shared.ColorWarning
	case status == "ERROR":
		fg = shared.ColorError
	}
	return lipgloss.NewStyle().Foreground(fg)
}

func operStatusStyleFn(status string) lipgloss.Style {
	var fg color.Color = shared.ColorFg
	switch status {
	case "ONLINE":
		fg = shared.ColorSuccess
	case "OFFLINE":
		fg = shared.ColorError
	}
	return lipgloss.NewStyle().Foreground(fg)
}

func memberStatusStyle(status string) lipgloss.Style {
	var fg color.Color = shared.ColorFg
	switch status {
	case "ONLINE":
		fg = shared.ColorSuccess
	case "OFFLINE", "ERROR":
		fg = shared.ColorError
	case "NO_MONITOR", "DISABLED":
		fg = shared.ColorMuted
	case "DRAINING":
		fg = shared.ColorWarning
	}
	return lipgloss.NewStyle().Foreground(fg)
}

// --- Data fetching ---

func (m Model) fetchDetail() tea.Cmd {
	client := m.client
	id := m.lbID
	return func() tea.Msg {
		shared.Debugf("[lbdetail] fetchDetail start")
		ctx := context.Background()

		lb, err := loadbalancer.GetLoadBalancer(ctx, client, id)
		if err != nil {
			shared.Debugf("[lbdetail] fetchDetail error: %v", err)
			return lbDetailErrMsg{err: err}
		}

		lstnrs, err := loadbalancer.ListListeners(ctx, client, id)
		if err != nil {
			return lbDetailErrMsg{err: err}
		}

		pls, err := loadbalancer.ListPools(ctx, client, id)
		if err != nil {
			return lbDetailErrMsg{err: err}
		}

		members := make(map[string][]loadbalancer.Member)
		mons := make(map[string]*loadbalancer.HealthMonitor)

		for _, p := range pls {
			mems, err := loadbalancer.ListMembers(ctx, client, p.ID)
			if err == nil {
				members[p.ID] = mems
			}

			if p.MonitorID != "" {
				mon, err := loadbalancer.GetHealthMonitor(ctx, client, p.MonitorID)
				if err == nil {
					mons[p.MonitorID] = mon
				}
			}
		}

		shared.Debugf("[lbdetail] fetchDetail done: %d listeners, %d pools", len(lstnrs), len(pls))
		return lbDetailLoadedMsg{
			lb:        lb,
			listeners: lstnrs,
			pools:     pls,
			members:   members,
			monitors:  mons,
		}
	}
}

// ForceRefresh triggers a manual reload of the load balancer detail.
func (m *Model) ForceRefresh() tea.Cmd {
	shared.Debugf("[lbdetail] ForceRefresh()")
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
	switch m.focus {
	case FocusListeners:
		return "\u2191\u2193 navigate \u2022 o toggle admin \u2022 tab switch pane \u2022 ^d delete \u2022 R refresh \u2022 esc back \u2022 ? help"
	case FocusPools:
		return "\u2191\u2193 select pool \u2022 ^h monitor \u2022 o toggle admin \u2022 tab switch pane \u2022 ^d delete \u2022 R refresh \u2022 esc back \u2022 ? help"
	case FocusMembers:
		return "\u2191\u2193 navigate \u2022 space select \u2022 x all \u2022 w drain \u2022 o admin \u2022 ^d delete \u2022 R refresh \u2022 esc back \u2022 ? help"
	default:
		return "o toggle admin \u2022 tab switch pane \u2022 ^d delete \u2022 R refresh \u2022 esc back \u2022 ? help"
	}
}
