package imageview

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
	"github.com/larkly/lazystack/internal/compute"
	img "github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/copypicker"
)

// FocusPane identifies a pane in the image view.
type FocusPane int

const (
	FocusSelector   FocusPane = iota
	FocusInfo
	FocusProperties
	FocusServers
)

const focusPaneCount = 4
const narrowThreshold = 80

var selectedRowStyle = lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)

// Column defines an image list column.
type Column struct {
	Title    string
	MinWidth int
	Flex     int
	Priority int
	Key      string
	width    int
	hidden   bool
}

func defaultColumns() []Column {
	return []Column{
		{Title: "Name", MinWidth: 10, Flex: 3, Priority: 0, Key: "name"},
		{Title: "Status", MinWidth: 14, Flex: 0, Priority: 0, Key: "status"},
		{Title: "Size", MinWidth: 8, Flex: 0, Priority: 0, Key: "size"},
		{Title: "Min Disk", MinWidth: 8, Flex: 0, Priority: 2, Key: "min_disk"},
		{Title: "Min RAM", MinWidth: 8, Flex: 0, Priority: 2, Key: "min_ram"},
		{Title: "Visibility", MinWidth: 10, Flex: 0, Priority: 1, Key: "visibility"},
		{Title: "Created", MinWidth: 12, Flex: 0, Priority: 1, Key: "created"},
	}
}

func computeWidths(columns []Column, totalWidth int) []Column {
	for i := range columns {
		columns[i].hidden = false
		columns[i].width = columns[i].MinWidth
	}

	maxPrio := 0
	for _, c := range columns {
		if c.Priority > maxPrio {
			maxPrio = c.Priority
		}
	}

	for prio := maxPrio; prio >= 0; prio-- {
		if fitsWidth(columns, totalWidth) {
			break
		}
		for i := range columns {
			if columns[i].Priority == prio {
				columns[i].hidden = true
			}
		}
	}

	gaps := -1
	totalMin := 0
	totalFlex := 0
	for _, c := range columns {
		if c.hidden {
			continue
		}
		gaps++
		totalMin += c.MinWidth
		totalFlex += c.Flex
	}
	if gaps < 0 {
		gaps = 0
	}

	available := totalWidth - 2 - gaps
	if available < 0 {
		available = 0
	}

	remaining := available - totalMin
	if remaining > 0 && totalFlex > 0 {
		for i := range columns {
			if columns[i].hidden || columns[i].Flex == 0 {
				continue
			}
			extra := remaining * columns[i].Flex / totalFlex
			columns[i].width += extra
		}
	}

	return columns
}

func fitsWidth(columns []Column, totalWidth int) bool {
	needed := 2
	first := true
	for _, c := range columns {
		if c.hidden {
			continue
		}
		if !first {
			needed++
		}
		first = false
		needed += c.MinWidth
	}
	return needed <= totalWidth
}

// Messages
type imagesLoadedMsg struct{ images []img.Image }
type imagesErrMsg struct{ err error }
type serversLoadedMsg struct{ servers []compute.Server }
type sortClearMsg struct{}

// Model is the combined image selector + detail view.
type Model struct {
	imageClient   *gophercloud.ServiceClient
	computeClient *gophercloud.ServiceClient

	// Selector state
	images         []img.Image
	columns        []Column
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

	// Servers using image
	servers       []compute.Server
	serversCursor int
	serversScroll int

	// Pane focus
	focus FocusPane

	// UI state
	width           int
	height          int
	loading         bool
	spinner         spinner.Model
	err             string
	refreshInterval time.Duration
}

// New creates an image view model.
func New(imageClient, computeClient *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Prompt = "/"
	ti.CharLimit = 64

	return Model{
		imageClient:     imageClient,
		computeClient:   computeClient,
		columns:         defaultColumns(),
		loading:         true,
		spinner:         s,
		searchInput:     ti,
		refreshInterval: refreshInterval,
		sortAsc:         true,
	}
}

// Init starts the initial fetch.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[imageview] Init()")
	return tea.Batch(m.spinner.Tick, m.fetchImages(), m.fetchServers())
}

// --- Public accessors ---

func (m Model) FocusedPane() FocusPane { return m.focus }
func (m Model) InSelector() bool       { return m.focus == FocusSelector }
func (m Model) InServers() bool        { return m.focus == FocusServers }

// SelectedImage returns the image under the selector cursor.
func (m Model) SelectedImage() *img.Image {
	visible := m.visibleImages()
	if m.cursor >= 0 && m.cursor < len(visible) {
		i := visible[m.cursor]
		return &i
	}
	return nil
}

// ImageID returns the selected image ID.
func (m Model) ImageID() string {
	if i := m.SelectedImage(); i != nil {
		return i.ID
	}
	return ""
}

// ImageName returns the selected image name.
func (m Model) ImageName() string {
	if i := m.SelectedImage(); i != nil {
		if i.Name != "" {
			return i.Name
		}
		return i.ID
	}
	return ""
}

// ImageStatus returns the selected image status.
func (m Model) ImageStatus() string {
	if i := m.SelectedImage(); i != nil {
		return i.Status
	}
	return ""
}

// SelectedServerID returns the server ID under the servers pane cursor.
func (m Model) SelectedServerID() string {
	srvs := m.imageServers()
	if m.serversCursor >= 0 && m.serversCursor < len(srvs) {
		return srvs[m.serversCursor].ID
	}
	return ""
}

// CopyEntries returns the title and copyable fields for the selected image.
func (m Model) CopyEntries() (string, []copypicker.Entry) {
	i := m.SelectedImage()
	if i == nil {
		return "", nil
	}
	b := copypicker.Builder{}
	b.Add("ID", i.ID).Add("Name", i.Name).Add("Checksum", i.Checksum).Add("Owner", i.Owner)
	if m.focus == FocusServers {
		b.Add("Attached server ID", m.SelectedServerID())
	}
	name := i.Name
	if name == "" {
		name = i.ID
	}
	return "Copy — image " + name, b.Entries()
}

// --- Update ---

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case imagesLoadedMsg:
		shared.Debugf("[imageview] loaded %d images", len(msg.images))
		var cursorID string
		if i := m.SelectedImage(); i != nil {
			cursorID = i.ID
		}
		m.loading = false
		m.images = msg.images
		m.err = ""
		m.sortImages()
		if cursorID != "" {
			for i, im := range m.visibleImages() {
				if im.ID == cursorID {
					m.cursor = i
					break
				}
			}
		}
		visible := m.visibleImages()
		if m.cursor >= len(visible) {
			m.cursor = max(0, len(visible)-1)
		}
		return m, nil

	case imagesErrMsg:
		shared.Debugf("[imageview] error: %v", msg.err)
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case serversLoadedMsg:
		m.servers = msg.servers
		return m, nil

	case shared.TickMsg:
		if m.loading {
			return m, nil
		}
		return m, tea.Batch(m.fetchImages(), m.fetchServers())

	case sortClearMsg:
		m.sortHighlight = false
		return m, nil

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
		m.columns = computeWidths(m.columns, m.width-4)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
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
			return m, nil
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
	}
	return m, nil
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.searchActive = false
		m.searchFilter = ""
		m.searchInput.SetValue("")
		m.searchInput.Blur()
		m.cursor = 0
		m.selectorScroll = 0
		return m, nil

	case key.Matches(msg, shared.Keys.Enter):
		m.searchActive = false
		m.searchFilter = m.searchInput.Value()
		m.searchInput.Blur()
		m.cursor = 0
		m.selectorScroll = 0
		visible := m.visibleImages()
		if m.cursor >= len(visible) {
			m.cursor = max(0, len(visible)-1)
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.searchFilter = m.searchInput.Value()
		m.cursor = 0
		m.selectorScroll = 0
		return m, cmd
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
	case FocusServers:
		m.serversCursor -= n
		if m.serversCursor < 0 {
			m.serversCursor = 0
		}
		m.ensureServersCursorVisible()
	}
	return m, nil
}

func (m Model) scrollDown(n int) (Model, tea.Cmd) {
	switch m.focus {
	case FocusSelector:
		visible := m.visibleImages()
		m.cursor += n
		maxIdx := len(visible) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.cursor > maxIdx {
			m.cursor = maxIdx
		}
		m.ensureSelectorCursorVisible()
	case FocusServers:
		srvs := m.imageServers()
		m.serversCursor += n
		maxIdx := len(srvs) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.serversCursor > maxIdx {
			m.serversCursor = maxIdx
		}
		m.ensureServersCursorVisible()
	}
	return m, nil
}

// --- Filter/search ---

func (m Model) visibleImages() []img.Image {
	if m.searchFilter == "" {
		return m.images
	}
	filter := strings.ToLower(m.searchFilter)
	var result []img.Image
	for _, i := range m.images {
		if strings.Contains(strings.ToLower(i.Name), filter) {
			result = append(result, i)
		}
	}
	return result
}

// --- Sort ---

func (m Model) cycleSort() (Model, tea.Cmd) {
	visCount := m.visibleColCount()
	if visCount == 0 {
		return m, nil
	}
	var cursorID string
	if i := m.SelectedImage(); i != nil {
		cursorID = i.ID
	}
	m.sortCol = (m.sortCol + 1) % visCount
	m.sortAsc = true
	m.sortHighlight = true
	m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
	m.sortImages()
	m.restoreCursor(cursorID)
	return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
		return sortClearMsg{}
	})
}

func (m Model) reverseSort() (Model, tea.Cmd) {
	if m.visibleColCount() == 0 {
		return m, nil
	}
	var cursorID string
	if i := m.SelectedImage(); i != nil {
		cursorID = i.ID
	}
	m.sortAsc = !m.sortAsc
	m.sortHighlight = true
	m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
	m.sortImages()
	m.restoreCursor(cursorID)
	return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
		return sortClearMsg{}
	})
}

func (m *Model) restoreCursor(cursorID string) {
	if cursorID == "" {
		return
	}
	for i, im := range m.visibleImages() {
		if im.ID == cursorID {
			m.cursor = i
			return
		}
	}
}

func (m Model) visibleColCount() int {
	n := 0
	for _, col := range m.columns {
		if !col.hidden {
			n++
		}
	}
	return n
}

func (m Model) visibleColKey(idx int) string {
	n := 0
	for _, col := range m.columns {
		if col.hidden {
			continue
		}
		if n == idx {
			return col.Key
		}
		n++
	}
	return ""
}

func (m *Model) sortImages() {
	if len(m.images) == 0 {
		return
	}
	colKey := m.visibleColKey(m.sortCol)
	if colKey == "" {
		return
	}
	asc := m.sortAsc
	sort.SliceStable(m.images, func(i, j int) bool {
		a, b := m.images[i], m.images[j]
		var less bool
		switch colKey {
		case "name":
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "status":
			less = a.Status < b.Status
		case "size":
			less = a.Size < b.Size
		case "min_disk":
			less = a.MinDisk < b.MinDisk
		case "min_ram":
			less = a.MinRAM < b.MinRAM
		case "visibility":
			less = a.Visibility < b.Visibility
		case "created":
			less = a.CreatedAt.Before(b.CreatedAt)
		default:
			less = false
		}
		if !asc {
			return !less
		}
		return less
	})
}

// --- Servers using image ---

func (m Model) imageServers() []compute.Server {
	sel := m.SelectedImage()
	if sel == nil {
		return nil
	}
	var result []compute.Server
	for _, s := range m.servers {
		if s.ImageID == sel.ID {
			result = append(result, s)
		}
	}
	return result
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

func (m *Model) ensureServersCursorVisible() {
	visH := m.serversVisibleLines()
	if m.serversCursor < m.serversScroll {
		m.serversScroll = m.serversCursor
	}
	if m.serversCursor >= m.serversScroll+visH {
		m.serversScroll = m.serversCursor - visH + 1
	}
}

// --- Height calculations ---

func (m Model) totalPanelHeight() int {
	h := m.height - 8
	if h < 10 {
		h = 10
	}
	return h
}

func (m Model) selectorHeight() int {
	visible := m.visibleImages()
	h := len(visible) + 6
	if h < 7 {
		h = 7
	}
	maxH := m.totalPanelHeight() * 40 / 100
	if maxH < 7 {
		maxH = 7
	}
	if h > maxH {
		h = maxH
	}
	return h
}

func (m Model) selectorVisibleLines() int {
	lines := m.selectorHeight() - 5
	if lines < 1 {
		lines = 1
	}
	return lines
}

func (m Model) detailHeight() int {
	return m.totalPanelHeight() - m.selectorHeight()
}

func (m Model) midHeight() int {
	h := m.detailHeight() * 55 / 100
	if h < 6 {
		h = 6
	}
	return h
}

func (m Model) bottomHeight() int {
	h := m.detailHeight() - m.midHeight()
	if h < 4 {
		h = 4
	}
	return h
}

func (m Model) serversVisibleLines() int {
	lines := m.bottomHeight() - 5
	if lines < 1 {
		lines = 1
	}
	return lines
}

// --- View ---

func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Images")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.images))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.images) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No images found.") + "\n")
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

	selContent := padContent(m.selectorTitle(), m.renderSelectorContent(m.width-4, selH-4))
	selPanel := m.panelBorder(FocusSelector).
		Width(m.width).
		Height(selH).
		Render(selContent)

	if m.SelectedImage() == nil {
		return selPanel
	}

	midH := m.midHeight()
	bottomH := m.bottomHeight()

	leftW := m.width * 35 / 100
	rightW := m.width - leftW - 1

	infoContent := padContent(m.panelTitle(FocusInfo), m.renderInfoContent(leftW-4))
	infoPanel := m.panelBorder(FocusInfo).Width(leftW).Height(midH).Render(infoContent)

	propsContent := padContent(m.panelTitle(FocusProperties), m.renderPropertiesContent(rightW-4))
	propsPanel := m.panelBorder(FocusProperties).Width(rightW).Height(midH).Render(propsContent)

	serversContent := padContent(m.panelTitle(FocusServers), m.renderServersContent(m.width-4, bottomH-4))
	serversPanel := m.panelBorder(FocusServers).Width(m.width).Height(bottomH).Render(serversContent)

	midRow := lipgloss.JoinHorizontal(lipgloss.Top, infoPanel, " ", propsPanel)

	return selPanel + "\n" + midRow + "\n" + serversPanel
}

func (m Model) renderNarrow() string {
	w := m.width - 2
	totalH := m.totalPanelHeight()

	selH := m.selectorHeight()
	remaining := totalH - selH

	selContent := m.renderSelectorContent(w-4, selH-4)
	selPanel := m.panelBorder(FocusSelector).Width(w).Height(selH).Render(padContent(m.selectorTitle(), selContent))

	if m.SelectedImage() == nil {
		return selPanel
	}

	infoH := remaining * 30 / 100
	propsH := remaining * 35 / 100
	serversH := remaining - infoH - propsH

	for _, h := range []*int{&infoH, &propsH, &serversH} {
		if *h < 4 {
			*h = 4
		}
	}

	infoPanel := m.panelBorder(FocusInfo).Width(w).Height(infoH).Render(padContent(m.panelTitle(FocusInfo), m.renderInfoContent(w-4)))
	propsPanel := m.panelBorder(FocusProperties).Width(w).Height(propsH).Render(padContent(m.panelTitle(FocusProperties), m.renderPropertiesContent(w-4)))
	serversPanel := m.panelBorder(FocusServers).Width(w).Height(serversH).Render(padContent(m.panelTitle(FocusServers), m.renderServersContent(w-4, serversH-4)))

	return lipgloss.JoinVertical(lipgloss.Left, selPanel, infoPanel, propsPanel, serversPanel)
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

	visible := m.visibleImages()
	t := titleStyle.Render("Images")

	if m.searchActive {
		t += " " + m.searchInput.View()
	} else if m.searchFilter != "" {
		filterStyle := lipgloss.NewStyle().Foreground(shared.ColorHighlight)
		t += " " + filterStyle.Render("/"+m.searchFilter)
		t += " " + shared.StyleHelp.Render(fmt.Sprintf("(%d/%d)", len(visible), len(m.images)))
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
	case FocusProperties:
		return titleStyle.Render("Properties")
	case FocusServers:
		srvs := m.imageServers()
		t := titleStyle.Render("Servers")
		if len(srvs) > 0 {
			t += " " + shared.StyleHelp.Render(fmt.Sprintf("(%d)", len(srvs)))
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
	visible := m.visibleImages()
	if len(visible) == 0 {
		if m.searchFilter != "" {
			return shared.StyleHelp.Render("No matches")
		}
		return ""
	}

	cols := computeWidths(defaultColumns(), maxWidth)

	// Header
	visIdx := 0
	var headerParts []string
	for _, col := range cols {
		if col.hidden {
			continue
		}
		title := col.Title
		idx := visIdx
		visIdx++
		indicator := ""
		if idx == m.sortCol {
			if m.sortAsc {
				indicator = " \u25b2"
			} else {
				indicator = " \u25bc"
			}
		}
		if idx == m.sortCol && m.sortHighlight {
			headerParts = append(headerParts, lipgloss.NewStyle().
				Width(col.width).
				Foreground(shared.ColorHighlight).
				Bold(true).
				Render(title+indicator))
		} else {
			headerParts = append(headerParts, shared.StyleHeader.Width(col.width).Render(title+indicator))
		}
	}
	headerLine := "  " + strings.Join(headerParts, " ")

	visibleLines := maxHeight - 1
	if visibleLines < 1 {
		visibleLines = 1
	}

	var lines []string
	lines = append(lines, headerLine)

	for i, im := range visible {
		if i < m.selectorScroll {
			continue
		}
		if i >= m.selectorScroll+visibleLines {
			break
		}

		selected := m.focus == FocusSelector && i == m.cursor
		isCursor := i == m.cursor

		name := im.Name
		if name == "" && len(im.ID) > 8 {
			name = im.ID[:8] + "..."
		}

		values := map[string]string{
			"name":       name,
			"status":     shared.StatusIcon(im.Status) + im.Status,
			"size":       shared.FormatSize(im.Size),
			"min_disk":   fmt.Sprintf("%dGB", im.MinDisk),
			"min_ram":    fmt.Sprintf("%dMB", im.MinRAM),
			"visibility": im.Visibility,
			"created":    im.CreatedAt.Format("2006-01-02"),
		}

		var rowBg color.Color
		hasBg := false
		if selected {
			rowBg = lipgloss.Color("#073642")
			hasBg = true
		}

		var parts []string
		for _, col := range cols {
			if col.hidden {
				continue
			}
			val := values[col.Key]
			w := col.width
			if len(val) > w && w > 1 {
				val = val[:w-1] + "\u2026"
			}

			style := lipgloss.NewStyle().Width(w)
			if col.Key == "status" {
				style = imageStatusStyle(im.Status).Width(w)
			}
			if isCursor {
				style = style.Bold(true)
			}
			if hasBg {
				style = style.Background(rowBg)
			}
			parts = append(parts, style.Render(val))
		}

		prefix := "  "
		prefixStyle := lipgloss.NewStyle()
		gapStyle := lipgloss.NewStyle()
		if hasBg {
			prefixStyle = prefixStyle.Background(rowBg)
			gapStyle = gapStyle.Background(rowBg)
		}

		gap := gapStyle.Render(" ")
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
	im := m.SelectedImage()
	if im == nil {
		return ""
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
		{"Name", im.Name, nil},
		{"ID", im.ID, nil},
		{"Status", im.Status, imageStatusStyleFn},
		{"Size", shared.FormatSize(im.Size), nil},
		{"Visibility", im.Visibility, nil},
		{"Owner", im.Owner, nil},
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

	// Summary
	srvs := m.imageServers()
	if len(srvs) > 0 {
		rows = append(rows, "")
		summaryStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
		noun := "servers"
		if len(srvs) == 1 {
			noun = "server"
		}
		rows = append(rows, summaryStyle.Render(fmt.Sprintf("%d %s using this image", len(srvs), noun)))
	}

	return strings.Join(rows, "\n")
}

// --- Properties rendering ---

func (m Model) renderPropertiesContent(maxWidth int) string {
	im := m.SelectedImage()
	if im == nil {
		return ""
	}

	labelW := 16
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(labelW)
	valueStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)

	protectedStr := "no"
	if im.Protected {
		protectedStr = "yes"
	}

	props := []struct {
		label string
		value string
	}{
		{"Disk Format", im.DiskFormat},
		{"Container", im.ContainerFormat},
		{"Checksum", im.Checksum},
		{"Min Disk", fmt.Sprintf("%d GB", im.MinDisk)},
		{"Min RAM", fmt.Sprintf("%d MB", im.MinRAM)},
		{"Protected", protectedStr},
		{"Tags", strings.Join(im.Tags, ", ")},
		{"Created", im.CreatedAt.Format("2006-01-02 15:04:05")},
		{"Updated", im.UpdatedAt.Format("2006-01-02 15:04:05")},
	}

	valW := maxWidth - labelW
	if valW < 4 {
		valW = 4
	}

	var rows []string
	for _, p := range props {
		if p.value == "" {
			continue
		}
		label := labelStyle.Render(p.label)
		val := p.value
		if lipgloss.Width(val) > valW {
			val = val[:valW-1] + "\u2026"
		}
		rows = append(rows, label+valueStyle.Render(val))
	}

	return strings.Join(rows, "\n")
}

// --- Servers rendering ---

func (m Model) renderServersContent(maxWidth, maxHeight int) string {
	srvs := m.imageServers()
	if m.SelectedImage() == nil {
		return ""
	}
	if len(srvs) == 0 {
		return shared.StyleHelp.Render("No servers using this image")
	}

	// Compact two-column mode when space is tight
	visibleLines := maxHeight - 1
	if visibleLines < 1 {
		visibleLines = 1
	}
	compact := visibleLines <= 3 && len(srvs) > visibleLines

	if compact {
		return m.renderServersCompact(srvs, maxWidth, visibleLines)
	}

	const gap = 2
	sep := strings.Repeat(" ", gap)

	nameW := len("Name")
	for _, s := range srvs {
		if len(s.Name) > nameW {
			nameW = len(s.Name)
		}
	}
	maxNameW := maxWidth / 3
	if maxNameW < 10 {
		maxNameW = 10
	}
	if nameW > maxNameW {
		nameW = maxNameW
	}

	headerStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	header := fmt.Sprintf("  %-*s%s%-10s%s%-20s%s%s",
		nameW, "Name", sep, "Status", sep, "Address", sep, "Flavor")
	headerLine := headerStyle.Render(header)

	var lines []string
	lines = append(lines, headerLine)

	for i, s := range srvs {
		if i < m.serversScroll {
			continue
		}
		if i >= m.serversScroll+visibleLines {
			break
		}

		selected := m.focus == FocusServers && i == m.serversCursor
		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		name := s.Name
		if len(name) > nameW {
			name = name[:nameW-1] + "\u2026"
		}

		// Prefer IPv6 address
		addr := ""
		if len(s.IPv6) > 0 {
			addr = s.IPv6[0]
		} else if len(s.IPv4) > 0 {
			addr = s.IPv4[0]
		}
		if len(addr) > 20 {
			addr = addr[:19] + "\u2026"
		}

		statusIcon := shared.StatusIcon(s.Status)
		statusStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if sc, ok := shared.StatusColors[s.Status]; ok {
			statusStyle = statusStyle.Foreground(sc)
		}

		line := fmt.Sprintf("%s%-*s%s%s%s%-20s%s%s",
			prefix, nameW, name, sep,
			statusStyle.Render(fmt.Sprintf("%-10s", statusIcon+s.Status)), sep,
			addr, sep, s.FlavorName)

		if selected {
			line = selectedRowStyle.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderServersCompact(srvs []compute.Server, maxWidth, visibleLines int) string {
	colW := maxWidth/2 - 2
	if colW < 20 {
		colW = 20
	}
	nameW := colW - 14 // room for status icon + status text
	if nameW < 8 {
		nameW = 8
	}

	// Two servers per line
	var lines []string
	totalSlots := visibleLines * 2
	start := m.serversScroll * 2 // scroll by pairs
	if start >= len(srvs) {
		start = 0
	}

	for row := 0; row < visibleLines; row++ {
		var cells []string
		for col := 0; col < 2; col++ {
			idx := start + row*2 + col
			if idx >= len(srvs) || idx >= start+totalSlots {
				cells = append(cells, strings.Repeat(" ", colW))
				continue
			}
			s := srvs[idx]
			selected := m.focus == FocusServers && idx == m.serversCursor

			prefix := "  "
			if selected {
				prefix = "\u25b8 "
			}

			name := s.Name
			if len(name) > nameW {
				name = name[:nameW-1] + "\u2026"
			}

			statusIcon := shared.StatusIcon(s.Status)
			statusStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
			if sc, ok := shared.StatusColors[s.Status]; ok {
				statusStyle = statusStyle.Foreground(sc)
			}

			cell := fmt.Sprintf("%s%-*s %s", prefix, nameW, name,
				statusStyle.Render(statusIcon+s.Status))

			if selected {
				cell = selectedRowStyle.Render(cell)
			}
			cells = append(cells, cell)
		}
		lines = append(lines, strings.Join(cells, "  "))
	}

	return strings.Join(lines, "\n")
}

// --- Action bar ---

func (m Model) renderActionBar() string {
	keyStyle := lipgloss.NewStyle().
		Foreground(shared.ColorHighlight).
		Background(shared.ColorSecondary).
		Bold(true).Padding(0, 0)
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)

	type btn struct{ key, label string }
	var buttons []btn

	switch m.focus {
	case FocusSelector:
		buttons = append(buttons, btn{"^n", "Upload Image"})
		if im := m.SelectedImage(); im != nil {
			if im.Status == "deactivated" {
				buttons = append(buttons, btn{"d", "Reactivate"})
			} else {
				buttons = append(buttons, btn{"d", "Deactivate"})
			}
			buttons = append(buttons, btn{"^d", "Delete"})
		}
		buttons = append(buttons, btn{"/", "Search"})
	case FocusInfo:
		buttons = append(buttons, btn{"enter", "Edit Image"})
		if im := m.SelectedImage(); im != nil {
			buttons = append(buttons, btn{"^d", "Delete"})
		}
	case FocusProperties:
		buttons = append(buttons, btn{"^g", "Download"})
	case FocusServers:
		if m.SelectedServerID() != "" {
			buttons = append(buttons, btn{"enter", "Server Detail"})
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

func imageStatusStyle(status string) lipgloss.Style {
	var fg color.Color = shared.ColorFg
	switch status {
	case "active":
		fg = shared.ColorSuccess
	case "saving":
		fg = shared.ColorWarning
	case "queued", "importing":
		fg = shared.ColorCyan
	case "deactivated", "killed":
		fg = shared.ColorError
	case "deleted", "pending_delete":
		fg = shared.ColorMuted
	}
	return lipgloss.NewStyle().Foreground(fg)
}

func imageStatusStyleFn(status string) lipgloss.Style {
	return imageStatusStyle(status)
}

// --- Data fetching ---

func (m Model) fetchImages() tea.Cmd {
	client := m.imageClient
	if client == nil {
		return func() tea.Msg {
			return imagesErrMsg{err: fmt.Errorf("image service not available")}
		}
	}
	return func() tea.Msg {
		shared.Debugf("[imageview] fetch images start")
		imgs, err := img.ListImages(context.Background(), client)
		if err != nil {
			shared.Debugf("[imageview] fetch images error: %v", err)
			return imagesErrMsg{err: err}
		}
		shared.Debugf("[imageview] fetch images done, count=%d", len(imgs))
		return imagesLoadedMsg{images: imgs}
	}
}

func (m Model) fetchServers() tea.Cmd {
	client := m.computeClient
	if client == nil {
		return nil
	}
	return func() tea.Msg {
		shared.Debugf("[imageview] fetch servers start")
		srvs, err := compute.ListServers(context.Background(), client)
		if err != nil {
			shared.Debugf("[imageview] fetch servers error (non-fatal): %v", err)
			return serversLoadedMsg{servers: nil}
		}
		shared.Debugf("[imageview] fetch servers done, count=%d", len(srvs))
		return serversLoadedMsg{servers: srvs}
	}
}

// ForceRefresh triggers a manual reload.
func (m *Model) ForceRefresh() tea.Cmd {
	shared.Debugf("[imageview] ForceRefresh()")
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchImages(), m.fetchServers())
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.columns = computeWidths(m.columns, w-4)
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	switch m.focus {
	case FocusSelector:
		return "\u2191\u2193 navigate \u2022 / search \u2022 S sort \u2022 ^n upload \u2022 d deactivate \u2022 ^d delete \u2022 tab switch pane \u2022 R refresh \u2022 ? help"
	case FocusInfo:
		return "enter edit \u2022 ^d delete \u2022 tab switch pane \u2022 R refresh \u2022 ? help"
	case FocusProperties:
		return "^g download \u2022 tab switch pane \u2022 R refresh \u2022 ? help"
	case FocusServers:
		return "\u2191\u2193 navigate \u2022 enter server detail \u2022 tab switch pane \u2022 R refresh \u2022 ? help"
	default:
		return "tab switch pane \u2022 R refresh \u2022 ? help"
	}
}
