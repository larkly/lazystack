package lbview

import (
	"context"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/copypicker"
)

// FocusPane identifies a pane in the load balancer view.
type FocusPane int

const (
	FocusSelector FocusPane = iota
	FocusInfo
	FocusListeners
	FocusPools
	FocusMembers
)

const focusPaneCount = 5
const narrowThreshold = 80

var (
	selectedRowStyle = lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)
	sortColumns      = []string{"name", "vipaddress", "provstatus", "operstatus"}
)

// Messages
type lbsLoadedMsg struct{ lbs []loadbalancer.LoadBalancer }
type lbsErrMsg struct{ err error }
type detailLoadedMsg struct {
	lbID      string
	listeners []loadbalancer.Listener
	pools     []loadbalancer.Pool
	members   map[string][]loadbalancer.Member
	monitors  map[string]*loadbalancer.HealthMonitor
}
type detailErrMsg struct {
	lbID string
	err  error
}
type sortClearMsg struct{}

// Model is the combined load balancer selector + detail view.
type Model struct {
	client *gophercloud.ServiceClient

	// Selector state
	lbs            []loadbalancer.LoadBalancer
	cursor         int
	selectorScroll int
	sortCol        int
	sortAsc        bool
	sortHighlight  bool
	sortClearAt    time.Time

	// Search/filter
	searchActive bool
	searchInput  textinput.Model
	searchFilter string

	// Detail state for selected LB
	listeners    []loadbalancer.Listener
	pools        []loadbalancer.Pool
	members      map[string][]loadbalancer.Member
	monitors     map[string]*loadbalancer.HealthMonitor
	lastDetailID string
	detailErr    string

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

	// UI state
	width           int
	height          int
	loading         bool
	detailLoading   bool
	spinner         spinner.Model
	err             string
	refreshInterval time.Duration

	// Adaptive polling state
	detailRefreshInterval time.Duration  // Current adaptive interval for detail fetches
	lastDetailFetch       time.Time      // When detail was last fetched
	pollMode              string         // Current polling mode label ("fast", "medium", "slow", "capped")
}

// New creates a load balancer view model.
func New(client *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Prompt = "/"
	ti.CharLimit = 64

	return Model{
		client:          client,
		loading:         true,
		spinner:         s,
		searchInput:     ti,
		members:         make(map[string][]loadbalancer.Member),
		monitors:        make(map[string]*loadbalancer.HealthMonitor),
		selectedMembers: make(map[string]bool),
		refreshInterval: refreshInterval,
		sortAsc:         true,
	}
}

// Init starts the initial fetch.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[lbview] Init()")
	return tea.Batch(m.spinner.Tick, m.fetchLBs())
}

// --- Public accessors ---

// FocusedPane returns the currently focused pane.
func (m Model) FocusedPane() FocusPane { return m.focus }

// InSelector returns true if the selector pane is focused.
func (m Model) InSelector() bool { return m.focus == FocusSelector }

// SelectedLB returns the load balancer under the selector cursor.
func (m Model) SelectedLB() *loadbalancer.LoadBalancer {
	visible := m.visibleLBs()
	if m.cursor >= 0 && m.cursor < len(visible) {
		lb := visible[m.cursor]
		return &lb
	}
	return nil
}

// LB returns the currently selected load balancer (alias for SelectedLB).
func (m Model) LB() *loadbalancer.LoadBalancer {
	return m.SelectedLB()
}

// LBID returns the selected load balancer ID.
func (m Model) LBID() string {
	if lb := m.SelectedLB(); lb != nil {
		return lb.ID
	}
	return ""
}

// LBName returns the selected load balancer name.
func (m Model) LBName() string {
	if lb := m.SelectedLB(); lb != nil {
		if lb.Name != "" {
			return lb.Name
		}
		return lb.ID
	}
	return ""
}

// CopyEntries returns the title and copyable fields for the selected
// load balancer, with extras for the focused listener/pool/member when
// one of those panes has focus.
func (m Model) CopyEntries() (string, []copypicker.Entry) {
	lb := m.SelectedLB()
	if lb == nil {
		return "", nil
	}
	b := copypicker.Builder{}
	b.Add("ID", lb.ID).Add("Name", lb.Name).Add("VIP Address", lb.VipAddress)
	switch m.focus {
	case FocusListeners:
		if l := m.SelectedListener(); l != nil {
			b.Add("Listener ID", l.ID).Add("Listener Name", l.Name)
		}
	case FocusPools:
		if p := m.SelectedPool(); p != nil {
			b.Add("Pool ID", p.ID).Add("Pool Name", p.Name)
		}
	case FocusMembers:
		if mem := m.SelectedMember(); mem != nil {
			b.Add("Member ID", mem.ID).Add("Member Name", mem.Name).Add("Member Address", mem.Address)
		}
	}
	name := lb.Name
	if name == "" {
		name = lb.ID
	}
	return "Copy — load balancer " + name, b.Entries()
}

// SelectedListenerID returns the ID of the currently selected listener.
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

// SelectedPoolID returns the ID of the currently selected pool.
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

// SelectedMemberID returns the ID of the currently selected member.
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
func (m Model) Listeners() []loadbalancer.Listener { return m.listeners }

// Pools returns the current pools list.
func (m Model) Pools() []loadbalancer.Pool { return m.pools }

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

// --- Update ---

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case lbsLoadedMsg:
		shared.Debugf("[lbview] loaded %d load balancers", len(msg.lbs))
		var cursorID string
		if lb := m.SelectedLB(); lb != nil {
			cursorID = lb.ID
		}
		m.loading = false
		m.lbs = msg.lbs
		m.err = ""
		m.sortLBs()
		// Restore cursor position
		if cursorID != "" {
			for i, lb := range m.visibleLBs() {
				if lb.ID == cursorID {
					m.cursor = i
					break
				}
			}
		}
		visible := m.visibleLBs()
		if m.cursor >= len(visible) {
			m.cursor = max(0, len(visible)-1)
		}
		// Trigger detail fetch if needed
		var cmd tea.Cmd
		if lb := m.SelectedLB(); lb != nil && lb.ID != m.lastDetailID {
			_, cmd = m.onSelectorChange()
		}
		return m, cmd

	case lbsErrMsg:
		shared.Debugf("[lbview] error: %v", msg.err)
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case detailLoadedMsg:
		shared.Debugf("[lbview] detail loaded: %d listeners, %d pools", len(msg.listeners), len(msg.pools))
		if lb := m.SelectedLB(); lb == nil || msg.lbID != lb.ID {
			return m, nil // stale detail
		}
		m.detailLoading = false
		m.listeners = msg.listeners
		m.pools = msg.pools
		m.members = msg.members
		m.monitors = msg.monitors
		m.detailErr = ""
		m.clampDetailCursors()
		return m, nil

	case detailErrMsg:
		if lb := m.SelectedLB(); lb == nil || msg.lbID != lb.ID {
			return m, nil
		}
		m.detailLoading = false
		m.detailErr = msg.err.Error()
		return m, nil

	case shared.TickMsg:
		if m.loading || m.detailLoading {
			return m, nil
		}
		cmds := []tea.Cmd{m.fetchLBs()}
		m, shouldRefresh := m.shouldRefreshDetail()
		if shouldRefresh {
			if lb := m.SelectedLB(); lb != nil {
				cmds = append(cmds, m.fetchDetail(lb.ID))
				m.lastDetailFetch = time.Now().UTC()
			}
		}
		return m, tea.Batch(cmds...)

	case sortClearMsg:
		m.sortHighlight = false
		return m, nil

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

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Search mode: route keys to text input
	if m.searchActive {
		return m.handleSearchKey(msg)
	}

	switch {
	case key.Matches(msg, shared.Keys.Back):
		if m.searchFilter != "" {
			m.searchFilter = ""
			m.searchInput.SetValue("")
			m.cursor = 0
			m.selectorScroll = 0
			return m.onSelectorChange()
		}
		return m, nil

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

	case key.Matches(msg, shared.Keys.Sort):
		if m.focus == FocusSelector {
			return m.cycleSort()
		}

	case key.Matches(msg, shared.Keys.ReverseSort):
		if m.focus == FocusSelector {
			return m.reverseSort()
		}

	case msg.String() == "/":
		if m.focus == FocusSelector {
			m.searchActive = true
			m.searchInput.SetValue(m.searchFilter)
			m.searchInput.Focus()
			return m, m.searchInput.Focus()
		}

	case key.Matches(msg, shared.Keys.Refresh):
		if !m.searchActive {
			return m, m.ForceRefresh()
		}
	}
	return m, nil
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		// Esc: clear filter and exit search
		m.searchActive = false
		m.searchFilter = ""
		m.searchInput.SetValue("")
		m.searchInput.Blur()
		m.cursor = 0
		m.selectorScroll = 0
		return m.onSelectorChange()

	case key.Matches(msg, shared.Keys.Enter):
		// Enter: keep filter and exit search
		m.searchActive = false
		m.searchFilter = m.searchInput.Value()
		m.searchInput.Blur()
		m.cursor = 0
		m.selectorScroll = 0
		visible := m.visibleLBs()
		if m.cursor >= len(visible) {
			m.cursor = max(0, len(visible)-1)
		}
		return m.onSelectorChange()

	default:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		// Live filter as user types
		m.searchFilter = m.searchInput.Value()
		m.cursor = 0
		m.selectorScroll = 0
		visible := m.visibleLBs()
		if m.cursor >= len(visible) {
			m.cursor = max(0, len(visible)-1)
		}
		var cmds []tea.Cmd
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		_, selCmd := m.onSelectorChange()
		if selCmd != nil {
			cmds = append(cmds, selCmd)
		}
		return m, tea.Batch(cmds...)
	}
}

// --- Scrolling ---

func (m Model) scrollUp(n int) (Model, tea.Cmd) {
	switch m.focus {
	case FocusSelector:
		m.cursor -= n
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureSelectorCursorVisible()
		return m.onSelectorChange()
	case FocusInfo:
		// no scrolling
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
	return m, nil
}

func (m Model) scrollDown(n int) (Model, tea.Cmd) {
	switch m.focus {
	case FocusSelector:
		visible := m.visibleLBs()
		m.cursor += n
		maxIdx := len(visible) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.cursor > maxIdx {
			m.cursor = maxIdx
		}
		m.ensureSelectorCursorVisible()
		return m.onSelectorChange()
	case FocusInfo:
		// no scrolling
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
	return m, nil
}

func (m Model) onSelectorChange() (Model, tea.Cmd) {
	lb := m.SelectedLB()
	if lb == nil {
		if m.lastDetailID != "" {
			m.resetDetailState()
			m.lastDetailID = ""
		}
		return m, nil
	}
	if lb.ID == m.lastDetailID {
		return m, nil
	}
	m.lastDetailID = lb.ID
	m.resetDetailState()
	m.detailLoading = true
	return m, tea.Batch(m.spinner.Tick, m.fetchDetail(lb.ID))
}

func (m *Model) resetDetailState() {
	m.listeners = nil
	m.pools = nil
	m.members = make(map[string][]loadbalancer.Member)
	m.monitors = make(map[string]*loadbalancer.HealthMonitor)
	m.detailErr = ""
	m.listenerCursor = 0
	m.listenerScroll = 0
	m.poolCursor = 0
	m.poolScroll = 0
	m.memberCursor = 0
	m.memberScroll = 0
	m.selectedMembers = make(map[string]bool)
}

// shouldRefreshDetail returns true if enough time has elapsed since the last
// detail fetch based on the selected LB's provisioning/operating status.
// Updates the adaptive interval and poll mode on the returned model.
func (m Model) shouldRefreshDetail() (Model, bool) {
	lb := m.SelectedLB()
	if lb == nil {
		return m, false
	}

	var interval time.Duration
	var mode string

	switch {
	case strings.HasPrefix(lb.ProvisioningStatus, "PENDING_"):
		interval = 2 * time.Second
		mode = "fast"
	case strings.HasPrefix(lb.OperatingStatus, "ERROR") || strings.HasPrefix(lb.OperatingStatus, "DEGRADED"):
		interval = 5 * time.Second
		mode = "medium"
	case lb.OperatingStatus == "ONLINE":
		interval = 15 * time.Second
		mode = "slow"
	case lb.OperatingStatus == "OFFLINE" || lb.OperatingStatus == "NO_MONITOR":
		interval = 10 * time.Second
		mode = "medium"
	default:
		// ACTIVE provisioning with unknown operating status
		if lb.ProvisioningStatus == "ACTIVE" {
			interval = 15 * time.Second
			mode = "slow"
		} else {
			interval = 5 * time.Second
			mode = "medium"
		}
	}

	m.pollMode = mode
	m.detailRefreshInterval = interval
	if m.lastDetailFetch.IsZero() || time.Since(m.lastDetailFetch) >= interval {
		return m, true
	}
	return m, false
}

// --- Filter/search ---

func (m Model) visibleLBs() []loadbalancer.LoadBalancer {
	if m.searchFilter == "" {
		return m.lbs
	}
	filter := strings.ToLower(m.searchFilter)
	var result []loadbalancer.LoadBalancer
	for _, lb := range m.lbs {
		if strings.Contains(strings.ToLower(lb.Name), filter) ||
			strings.Contains(strings.ToLower(lb.VipAddress), filter) {
			result = append(result, lb)
		}
	}
	return result
}

// --- Sort ---

func (m Model) cycleSort() (Model, tea.Cmd) {
	var cursorID string
	if lb := m.SelectedLB(); lb != nil {
		cursorID = lb.ID
	}
	m.sortCol = (m.sortCol + 1) % len(sortColumns)
	m.sortAsc = true
	m.sortHighlight = true
	m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
	m.sortLBs()
	m.restoreCursor(cursorID)
	return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
		return sortClearMsg{}
	})
}

func (m Model) reverseSort() (Model, tea.Cmd) {
	var cursorID string
	if lb := m.SelectedLB(); lb != nil {
		cursorID = lb.ID
	}
	m.sortAsc = !m.sortAsc
	m.sortHighlight = true
	m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
	m.sortLBs()
	m.restoreCursor(cursorID)
	return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
		return sortClearMsg{}
	})
}

func (m *Model) restoreCursor(cursorID string) {
	if cursorID == "" {
		return
	}
	for i, lb := range m.visibleLBs() {
		if lb.ID == cursorID {
			m.cursor = i
			return
		}
	}
}

func (m *Model) sortLBs() {
	if len(m.lbs) == 0 {
		return
	}
	colKey := sortColumns[m.sortCol]
	asc := m.sortAsc
	sort.SliceStable(m.lbs, func(i, j int) bool {
		a, b := m.lbs[i], m.lbs[j]
		var less bool
		switch colKey {
		case "name":
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "vipaddress":
			less = a.VipAddress < b.VipAddress
		case "provstatus":
			less = a.ProvisioningStatus < b.ProvisioningStatus
		case "operstatus":
			less = a.OperatingStatus < b.OperatingStatus
		default:
			less = false
		}
		if !asc {
			return !less
		}
		return less
	})
}

// --- Cursor visibility ---

func (m *Model) ensureSelectorCursorVisible() {
	visH := m.selectorVisibleLines()
	if m.cursor < m.selectorScroll {
		m.selectorScroll = m.cursor
	}
	if m.cursor >= m.selectorScroll+visH {
		m.selectorScroll = m.cursor - visH + 1
	}
}

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

func (m *Model) clampDetailCursors() {
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

// --- Height calculations ---

func (m Model) totalPanelHeight() int {
	h := m.height - 8 // title + blank + action bar + spacer + status bar + margins
	if h < 10 {
		h = 10
	}
	return h
}

func (m Model) selectorHeight() int {
	visible := m.visibleLBs()
	h := len(visible) + 4 // content + header + border
	if h < 5 {
		h = 5
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
	lines := m.selectorHeight() - 5 // border(4) + header(1)
	if lines < 1 {
		lines = 1
	}
	return lines
}

func (m Model) detailHeight() int {
	return m.totalPanelHeight() - m.selectorHeight()
}

func (m Model) topHeight() int {
	h := m.detailHeight() * 45 / 100
	if h < 6 {
		h = 6
	}
	return h
}

func (m Model) bottomHeight() int {
	h := m.detailHeight() - m.topHeight()
	if h < 6 {
		h = 6
	}
	return h
}

func (m Model) topVisibleLines() int {
	lines := m.topHeight() - 5
	if lines < 1 {
		lines = 1
	}
	return lines
}

func (m Model) bottomVisibleLines() int {
	lines := m.bottomHeight() - 5
	if lines < 1 {
		lines = 1
	}
	return lines
}

// --- View ---

func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Load Balancers")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.lbs))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.lbs) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No load balancers found.") + "\n")
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

	// Selector: full width
	selContent := padContent(m.selectorTitle(), m.renderSelectorContent(m.width-4, selH-4))
	selPanel := m.panelBorder(FocusSelector).
		Width(m.width).
		Height(selH).
		Render(selContent)

	if m.SelectedLB() == nil {
		return selPanel
	}

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

	return selPanel + "\n" + topRow + "\n" + bottomRow
}

func (m Model) renderNarrow() string {
	w := m.width - 2
	totalH := m.totalPanelHeight()

	selH := m.selectorHeight()
	remaining := totalH - selH

	selContent := m.renderSelectorContent(w-4, selH-4)
	selPanel := m.panelBorder(FocusSelector).Width(w).Height(selH).Render(padContent(m.selectorTitle(), selContent))

	if m.SelectedLB() == nil {
		return selPanel
	}

	infoH := remaining * 20 / 100
	listenersH := remaining * 20 / 100
	poolsH := remaining * 25 / 100
	membersH := remaining - infoH - listenersH - poolsH

	for _, h := range []*int{&infoH, &listenersH, &poolsH, &membersH} {
		if *h < 4 {
			*h = 4
		}
	}

	infoPanel := m.panelBorder(FocusInfo).Width(w).Height(infoH).Render(padContent(m.panelTitle(FocusInfo), m.renderInfoContent(w-4)))
	listenersPanel := m.panelBorder(FocusListeners).Width(w).Height(listenersH).Render(padContent(m.panelTitle(FocusListeners), m.renderListenersContent(w-4, listenersH-4)))
	poolsPanel := m.panelBorder(FocusPools).Width(w).Height(poolsH).Render(padContent(m.panelTitle(FocusPools), m.renderPoolsContent(w-4, poolsH-4)))
	membersPanel := m.panelBorder(FocusMembers).Width(w).Height(membersH).Render(padContent(m.panelTitle(FocusMembers), m.renderMembersContent(w-4, membersH-4)))

	return lipgloss.JoinVertical(lipgloss.Left, selPanel, infoPanel, listenersPanel, poolsPanel, membersPanel)
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

func (m Model) selectorTitle() string {
	borderColor := shared.ColorMuted
	if m.focus == FocusSelector {
		borderColor = shared.ColorPrimary
	}
	titleStyle := lipgloss.NewStyle().Foreground(borderColor).Bold(true)

	visible := m.visibleLBs()
	t := titleStyle.Render("Load Balancers")

	if m.searchActive {
		t += " " + m.searchInput.View()
	} else if m.searchFilter != "" {
		filterStyle := lipgloss.NewStyle().Foreground(shared.ColorHighlight)
		t += " " + filterStyle.Render("/"+m.searchFilter)
		t += " " + shared.StyleHelp.Render(fmt.Sprintf("(%d/%d)", len(visible), len(m.lbs)))
	}

	if m.loading {
		t += " " + m.spinner.View()
	}
	return t
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
		if m.detailLoading {
			t += " " + m.spinner.View()
		}
		return t
	case FocusPools:
		t := titleStyle.Render("Pools")
		if m.detailLoading {
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
		if m.detailLoading {
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

// --- Selector rendering ---

func (m Model) renderSelectorContent(maxWidth, maxHeight int) string {
	visible := m.visibleLBs()
	if len(visible) == 0 {
		if m.searchFilter != "" {
			return shared.StyleHelp.Render("No matches")
		}
		return ""
	}

	// Column widths
	nameW := len("Name")
	for _, lb := range visible {
		if len(lb.Name) > nameW {
			nameW = len(lb.Name)
		}
	}
	fixedW := 18 + 18 + 16 + 6 + 2 // vip + prov + oper + gaps + prefix
	maxNameW := maxWidth - fixedW
	if maxNameW < 12 {
		maxNameW = 12
	}
	if nameW > maxNameW {
		nameW = maxNameW
	}

	const gap = 2
	sep := strings.Repeat(" ", gap)

	// Header
	headerTitles := []struct {
		title string
		width int
	}{
		{"Name", nameW},
		{"VIP Address", 18},
		{"Prov. Status", 18},
		{"Oper. Status", 16},
	}
	var headerParts []string
	for i, h := range headerTitles {
		title := h.title
		indicator := ""
		if i == m.sortCol {
			if m.sortAsc {
				indicator = " \u25b2"
			} else {
				indicator = " \u25bc"
			}
		}
		if i == m.sortCol && m.sortHighlight {
			headerParts = append(headerParts, lipgloss.NewStyle().
				Foreground(shared.ColorHighlight).Bold(true).
				Render(fmt.Sprintf("%-*s", h.width, title+indicator)))
		} else {
			headerParts = append(headerParts, shared.StyleHeader.Render(fmt.Sprintf("%-*s", h.width, title+indicator)))
		}
	}
	headerLine := "  " + strings.Join(headerParts, sep)

	visibleLines := maxHeight - 1 // minus header
	if visibleLines < 1 {
		visibleLines = 1
	}

	var lines []string
	lines = append(lines, headerLine)

	for i, lb := range visible {
		if i < m.selectorScroll {
			continue
		}
		if i >= m.selectorScroll+visibleLines {
			break
		}

		selected := m.focus == FocusSelector && i == m.cursor
		prefix := "  "
		if i == m.cursor {
			prefix = "\u25b8 "
		}

		name := lb.Name
		if name == "" && len(lb.ID) > 8 {
			name = lb.ID[:8] + "..."
		}
		if len(name) > nameW {
			name = name[:nameW-1] + "\u2026"
		}

		provIcon := shared.StatusIcon(lb.ProvisioningStatus)
		operIcon := shared.StatusIcon(lb.OperatingStatus)

		provStyle := provStatusStyle(lb.ProvisioningStatus)
		operStyle := operStatusStyle(lb.OperatingStatus)

		nameStyle := lipgloss.NewStyle().Width(nameW)
		vipStyle := lipgloss.NewStyle().Width(18)
		psStyle := provStyle.Width(18)
		osStyle := operStyle.Width(16)

		var rowBg color.Color
		hasBg := false
		if selected {
			rowBg = lipgloss.Color("#073642")
			hasBg = true
			nameStyle = nameStyle.Bold(true).Background(rowBg)
			vipStyle = vipStyle.Bold(true).Background(rowBg)
			psStyle = psStyle.Bold(true).Background(rowBg)
			osStyle = osStyle.Bold(true).Background(rowBg)
		}

		parts := []string{
			nameStyle.Render(truncate(name, nameW)),
			vipStyle.Render(truncate(lb.VipAddress, 18)),
			psStyle.Render(provIcon + truncate(lb.ProvisioningStatus, 16)),
			osStyle.Render(operIcon + truncate(lb.OperatingStatus, 14)),
		}

		prefixStyle := lipgloss.NewStyle()
		gapStyle := lipgloss.NewStyle()
		if hasBg {
			prefixStyle = prefixStyle.Background(rowBg)
			gapStyle = gapStyle.Background(rowBg)
		}

		gap := gapStyle.Render(sep)
		row := prefixStyle.Render(prefix) + strings.Join(parts, gap)

		if hasBg {
			rowW := lipgloss.Width(row)
			if rowW < maxWidth+2 {
				row += gapStyle.Render(strings.Repeat(" ", maxWidth+2-rowW))
			}
		}

		lines = append(lines, row)
	}

	return strings.Join(lines, "\n")
}

// --- Info rendering ---

func (m Model) renderInfoContent(maxWidth int) string {
	lb := m.SelectedLB()
	if lb == nil {
		return ""
	}

	if m.detailLoading {
		return m.spinner.View() + " Loading..."
	}

	if m.detailErr != "" {
		return lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.detailErr)
	}

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

	// Poll status line
	if !m.lastDetailFetch.IsZero() && m.pollMode != "" {
		pollStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
		intervalStr := m.detailRefreshInterval.Round(time.Second).String()
		when := m.lastDetailFetch.Local().Format("15:04:05")
		rows = append(rows, pollStyle.Render(fmt.Sprintf("Poll: %s (every %s) · Refreshed: %s", m.pollMode, intervalStr, when)))
	}

	return strings.Join(rows, "\n")
}

// --- Listeners rendering ---

func (m Model) renderListenersContent(maxWidth, maxHeight int) string {
	if m.SelectedLB() == nil {
		return ""
	}
	if m.detailLoading {
		return m.spinner.View() + " Loading..."
	}
	if len(m.listeners) == 0 {
		return shared.StyleHelp.Render("No listeners configured")
	}

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
	if m.SelectedLB() == nil {
		return ""
	}
	if m.detailLoading {
		return m.spinner.View() + " Loading..."
	}
	if len(m.pools) == 0 {
		return shared.StyleHelp.Render("No pools configured")
	}

	nameW := len("Pool")
	for _, p := range m.pools {
		if len(p.Name) > nameW {
			nameW = len(p.Name)
		}
	}
	maxNameW := maxWidth - 2 - 14
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

	monReserve := 0
	if m.selectedPoolMonitor() != nil {
		monReserve = 8
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

		health := "\u2014"
		if mon := m.monitors[p.MonitorID]; mon != nil {
			health = mon.Type
			if mon.Type == "HTTP" || mon.Type == "HTTPS" {
				health = mon.Type + " " + mon.URLPath
			}
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

	// Health monitor details
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

		remaining := monReserve - 2
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
	if m.SelectedLB() == nil {
		return ""
	}
	if m.detailLoading {
		return m.spinner.View() + " Loading..."
	}
	members := m.selectedPoolMembers()
	if len(m.pools) == 0 {
		return shared.StyleHelp.Render("No pools to show members for")
	}
	if len(members) == 0 {
		return shared.StyleHelp.Render("No members in this pool")
	}

	const gap = 2
	sep := strings.Repeat(" ", gap)

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
	maxNameW := maxWidth - addrW - 6 - 12 - gap*3 - 2
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
	if m.SelectedLB() == nil && m.focus != FocusSelector {
		return ""
	}

	// Show PENDING status instead of actions when LB is provisioning
	if lb := m.SelectedLB(); lb != nil && strings.HasPrefix(lb.ProvisioningStatus, "PENDING_") {
		return " " + lipgloss.NewStyle().Foreground(shared.ColorWarning).Render(
			"\u25b2 "+lb.ProvisioningStatus+" \u2014 operations disabled")
	}

	keyStyle := lipgloss.NewStyle().
		Foreground(shared.ColorHighlight).
		Background(shared.ColorSecondary).
		Bold(true).Padding(0, 0)
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)

	type btn struct{ key, label string }
	var buttons []btn

	switch m.focus {
	case FocusSelector:
		buttons = append(buttons, btn{"^n", "Create LB"})
		if m.SelectedLB() != nil {
			buttons = append(buttons, btn{"^d", "Delete LB"})
		}
		buttons = append(buttons, btn{"/", "Search"})
	case FocusInfo:
		buttons = append(buttons, btn{"enter", "Edit LB"})
		if lb := m.SelectedLB(); lb != nil {
			if lb.AdminStateUp {
				buttons = append(buttons, btn{"o", "Disable"})
			} else {
				buttons = append(buttons, btn{"o", "Enable"})
			}
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

func truncate(s string, w int) string {
	if len(s) > w && w > 3 {
		return s[:w-3] + "..."
	}
	if len(s) > w {
		return s[:w]
	}
	return s
}

func provStatusStyle(status string) lipgloss.Style {
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

func operStatusStyle(status string) lipgloss.Style {
	var fg color.Color = shared.ColorFg
	switch status {
	case "ONLINE":
		fg = shared.ColorSuccess
	case "OFFLINE":
		fg = shared.ColorError
	}
	return lipgloss.NewStyle().Foreground(fg)
}

func provStatusStyleFn(status string) lipgloss.Style {
	return provStatusStyle(status)
}

func operStatusStyleFn(status string) lipgloss.Style {
	return operStatusStyle(status)
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

func (m Model) fetchLBs() tea.Cmd {
	client := m.client
	if client == nil {
		return func() tea.Msg {
			return lbsErrMsg{err: fmt.Errorf("load balancer service not available")}
		}
	}
	return func() tea.Msg {
		shared.Debugf("[lbview] fetch LBs start")
		lbs, err := loadbalancer.ListLoadBalancers(context.Background(), client)
		if err != nil {
			shared.Debugf("[lbview] fetch LBs error: %v", err)
			return lbsErrMsg{err: err}
		}
		shared.Debugf("[lbview] fetch LBs done, count=%d", len(lbs))
		return lbsLoadedMsg{lbs: lbs}
	}
}

func (m Model) fetchDetail(lbID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		shared.Debugf("[lbview] fetchDetail start for %s", lbID)
		ctx := context.Background()

		lb, err := loadbalancer.GetLoadBalancer(ctx, client, lbID)
		if err != nil {
			return detailErrMsg{lbID: lbID, err: err}
		}
		_ = lb // we already have it in the selector list

		lstnrs, err := loadbalancer.ListListeners(ctx, client, lbID)
		if err != nil {
			return detailErrMsg{lbID: lbID, err: err}
		}

		pls, err := loadbalancer.ListPools(ctx, client, lbID)
		if err != nil {
			return detailErrMsg{lbID: lbID, err: err}
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

		shared.Debugf("[lbview] fetchDetail done: %d listeners, %d pools", len(lstnrs), len(pls))
		return detailLoadedMsg{
			lbID:      lbID,
			listeners: lstnrs,
			pools:     pls,
			members:   members,
			monitors:  mons,
		}
	}
}

// ForceRefresh triggers a manual reload.
func (m *Model) ForceRefresh() tea.Cmd {
	shared.Debugf("[lbview] ForceRefresh()")
	m.loading = true
	cmds := []tea.Cmd{m.spinner.Tick, m.fetchLBs()}
	if lb := m.SelectedLB(); lb != nil {
		m.detailLoading = true
		cmds = append(cmds, m.fetchDetail(lb.ID))
	}
	m.lastDetailFetch = time.Now().UTC()
	// Reset adaptive interval so next TickMsg recalculates
	m.detailRefreshInterval = 0
	return tea.Batch(cmds...)
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	switch m.focus {
	case FocusSelector:
		return "\u2191\u2193 navigate \u2022 / search \u2022 S sort \u2022 ^n create \u2022 ^d delete \u2022 tab switch pane \u2022 R refresh \u2022 esc back \u2022 ? help"
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
