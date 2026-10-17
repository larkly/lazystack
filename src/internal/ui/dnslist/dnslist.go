package dnslist

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/filterbar"
)

type zonesLoadedMsg struct {
	seq   uint64
	zones []zones.Zone
}

type zonesErrMsg struct {
	seq uint64
	err error
}

type recordsetsLoadedMsg struct {
	zoneID     string
	recordsets []recordsets.RecordSet
}

type recordsetsErrMsg struct {
	zoneID string
	err    error
}

type sortClearMsg struct{}

// zoneSortColumns are the zone list's sort keys, cycled with s.
var zoneSortColumns = []string{"name", "type"}

// Model is the DNS zone/record list viewer.
type Model struct {
	client       *gophercloud.ServiceClient
	allZones     []zones.Zone // every zone, as last fetched
	zones        []zones.Zone // the listed zones: filtered and sorted
	filter       filterbar.Model
	order        shared.SortState
	recordsets   []recordsets.RecordSet
	selectedZone *zones.Zone
	cursor       int
	scroll       int
	width        int
	height       int
	loading      bool
	spinner      spinner.Model
	err          error
	refresh      shared.RefreshGate // zone list fetches
	needRecords  bool               // Escape changed the selected zone; load its records
}

// New creates a DNS list model.
func New(client *gophercloud.ServiceClient) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:  client,
		loading: true,
		spinner: s,
		filter:  filterbar.New(),
		order:   shared.NewSortState(),
	}
}

// IsFiltering reports whether the "/" zone filter input has focus.
func (m Model) IsFiltering() bool { return m.filter.Active() }

// FilterValue is the current zone filter text.
func (m Model) FilterValue() string { return m.filter.Value() }

// Escape clears the committed filter and reports whether there was one;
// when there was not, esc goes on to mean back-navigation in the root model.
func (m *Model) Escape() (handled bool) {
	if !m.filter.Clear() {
		return false
	}
	if m.applyFilter() {
		m.needRecords = true
	}
	return true
}

// syncRecords loads the selected zone's records when Escape changed the
// selection.
func (m *Model) syncRecords() tea.Cmd {
	changed := m.needRecords
	m.needRecords = false
	return m.recordsFor(changed)
}

// Init fetches zones.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchZones(m.refresh.Seq()))
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case zonesLoadedMsg:
		if !m.refresh.Accept(msg.seq) {
			return m, nil
		}
		m.loading = false
		m.allZones = msg.zones
		m.err = nil
		m.applyFilter()
		if m.selectedZone == nil {
			return m, nil
		}
		// Refresh the selected zone's records along with the zone list.
		return m, m.fetchRecordsets(m.selectedZone.ID)

	case zonesErrMsg:
		if !m.refresh.Accept(msg.seq) {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		m.scrollTo(m.cursor) // the error line takes a zone row
		return m, nil

	case recordsetsLoadedMsg:
		if !m.isSelectedZone(msg.zoneID) {
			// Late response for a zone the cursor has already left.
			return m, nil
		}
		m.loading = false
		m.recordsets = msg.recordsets
		m.err = nil
		return m, nil

	case recordsetsErrMsg:
		if !m.isSelectedZone(msg.zoneID) {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		m.scrollTo(m.cursor) // the error line takes a zone row
		return m, nil

	case shared.TickMsg:
		if m.loading || m.refresh.Busy() {
			return m, nil
		}
		seq := m.refresh.Start()
		return m, m.fetchZones(seq)

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case sortClearMsg:
		m.order.Highlight = false
		return m, nil

	case shared.SelectionChangedMsg:
		cmd := m.syncRecords()
		return m, cmd

	case tea.KeyMsg:
		if m.filter.Active() {
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			changed := m.applyFilter()
			records := m.recordsFor(changed)
			return m, tea.Batch(cmd, records)
		}
		switch {
		case key.Matches(msg, shared.Keys.Filter):
			return m, m.filter.Start()
		case key.Matches(msg, shared.Keys.Sort):
			m.order = shared.NextSort(m.order, len(zoneSortColumns))
			return m.resort()
		case key.Matches(msg, shared.Keys.ReverseSort):
			m.order = shared.ReverseSort(m.order)
			return m.resort()
		case key.Matches(msg, shared.Keys.Back):
			m.Escape()
			cmd := m.syncRecords()
			return m, cmd
		case key.Matches(msg, shared.Keys.Up):
			return m.moveCursor(m.cursor - 1)
		case key.Matches(msg, shared.Keys.Down):
			return m.moveCursor(m.cursor + 1)
		case key.Matches(msg, shared.Keys.PageUp):
			return m.moveCursor(m.cursor - m.visibleRows())
		case key.Matches(msg, shared.Keys.PageDown):
			return m.moveCursor(m.cursor + m.visibleRows())
		}
	}
	return m, nil
}

// rowIDs lists the IDs of the listed zones in display order.
func (m Model) rowIDs() []string {
	ids := make([]string, len(m.zones))
	for i, z := range m.zones {
		ids[i] = z.ID
	}
	return ids
}

// applyFilter rebuilds the listed zones from every zone under the current
// filter and sort and keeps the selection on the same zone while it is
// still listed; otherwise the cursor stays at the same position, clamped.
// It reports whether the selected zone changed, so the caller fetches the
// new zone's records.
func (m *Model) applyFilter() (changed bool) {
	prevID := ""
	if m.selectedZone != nil {
		prevID = m.selectedZone.ID
	}
	m.zones = m.zones[:0:0]
	for _, z := range m.allZones {
		if m.filter.Match(z.Name, z.Type, z.ID, z.Email, z.Description) {
			m.zones = append(m.zones, z)
		}
	}
	m.sortZones()
	if len(m.zones) == 0 {
		m.cursor, m.scroll = 0, 0
		m.selectedZone = nil
		m.recordsets = nil
		return prevID != ""
	}
	if i := shared.RestoreCursor(m.rowIDs(), prevID); i >= 0 {
		m.cursor = i
	}
	m.cursor = shared.ClampCursor(m.cursor, len(m.zones))
	m.scroll = min(m.scroll, max(0, len(m.zones)-m.visibleRows()))
	m.scrollTo(m.cursor)
	m.selectedZone = &m.zones[m.cursor]
	if m.selectedZone.ID != prevID {
		m.recordsets = nil
		return true
	}
	return false
}

// recordsFor starts loading the selected zone's records when the selection
// changed.
func (m *Model) recordsFor(changed bool) tea.Cmd {
	if !changed || m.selectedZone == nil {
		return nil
	}
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchRecordsets(m.selectedZone.ID))
}

func (m *Model) sortZones() {
	col := zoneSortColumns[m.order.Col]
	asc := m.order.Asc
	sort.SliceStable(m.zones, func(i, j int) bool {
		a, b := m.zones[i], m.zones[j]
		// Descending swaps the operands so equal keys keep their order.
		if !asc {
			a, b = b, a
		}
		if col == "type" {
			return zoneType(a) < zoneType(b)
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

// resort re-sorts the zones after a sort change, keeping the same zone
// selected, and highlights the sort column for a moment.
func (m Model) resort() (Model, tea.Cmd) {
	if m.selectedZone != nil {
		// selectedZone points into m.zones, so read its ID before sorting.
		id := m.selectedZone.ID
		m.sortZones()
		if i := shared.RestoreCursor(m.rowIDs(), id); i >= 0 {
			m.cursor = i
			m.selectedZone = &m.zones[i]
		}
		m.scrollTo(m.cursor)
	} else {
		m.sortZones()
	}
	return m, shared.SortHighlightCmd(sortClearMsg{})
}

// zoneType is the zone's type, PRIMARY when the API leaves it empty.
func zoneType(z zones.Zone) string {
	if z.Type != "" {
		return z.Type
	}
	return "PRIMARY"
}

// moveCursor clamps target to the zone list, keeps it visible and fetches
// the selected zone's records when the selection changes. Empty lists (not
// yet loaded, empty, or failed) are a no-op.
func (m Model) moveCursor(target int) (Model, tea.Cmd) {
	if len(m.zones) == 0 {
		m.cursor = 0
		m.scroll = 0
		return m, nil
	}
	target = max(0, min(target, len(m.zones)-1))
	m.scrollTo(target)
	if target == m.cursor && m.selectedZone != nil {
		return m, nil
	}
	m.cursor = target
	m.selectedZone = &m.zones[m.cursor]
	m.loading = true
	return m, tea.Batch(m.spinner.Tick, m.fetchRecordsets(m.zones[m.cursor].ID))
}

// scrollTo adjusts the scroll offset so that zone row i is visible.
func (m *Model) scrollTo(i int) {
	visible := m.visibleRows()
	if i < m.scroll {
		m.scroll = i
	}
	if i >= m.scroll+visible {
		m.scroll = i - visible + 1
	}
	m.scroll = max(0, m.scroll)
}

func (m Model) visibleRows() int {
	h := m.height - 6 // title + status bar + header + zone header + padding
	if m.err != nil {
		h-- // error line under the title
	}
	if m.filter.Visible() {
		h-- // filter line
	}
	if h < 1 {
		h = 1
	}
	return h
}

// View renders the DNS zone list with records.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("DNS Zones")
	if m.loading {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n")
	if m.err != nil {
		b.WriteString(shared.LoadErrorLine(m.err, m.width) + "\n")
	}
	if bar := m.filter.View(); bar != "" {
		b.WriteString(bar + "\n")
	}

	if len(m.zones) == 0 && (!m.loading || m.filter.Value() != "") {
		msg := "  No DNS zones found."
		if m.filter.Value() != "" {
			msg = "  No DNS zones match the filter."
		}
		b.WriteString(shared.StyleHelp.Render(msg) + "\n")
		return b.String()
	}

	// Zone list header, with the sort column's direction
	header := lipgloss.NewStyle().Bold(true).Foreground(shared.ColorMuted)
	sortHL := lipgloss.NewStyle().Bold(true).Foreground(shared.ColorHighlight)
	headerCell := func(col int, title string, w int) string {
		cell := fmt.Sprintf("%-*s", w, title+m.order.Indicator(col))
		if col == m.order.Col && m.order.Highlight {
			return sortHL.Render(cell)
		}
		return header.Render(cell)
	}
	b.WriteString(header.Render("  ") + headerCell(0, "Zone Name", 40) + header.Render(" ") +
		headerCell(1, "Type", 10) + header.Render(fmt.Sprintf(" %-12s", "Records")) + "\n")

	// Show zone list with record count summary
	visible := m.visibleRows()
	end := m.scroll + visible
	if end > len(m.zones) {
		end = len(m.zones)
	}

	for i := m.scroll; i < end; i++ {
		z := m.zones[i]
		prefix := "  "
		rowStyle := lipgloss.NewStyle()
		if i == m.cursor {
			prefix = "▶ "
			rowStyle = rowStyle.Background(shared.ColorHighlight).Foreground(shared.ColorFg)
		}

		row := fmt.Sprintf("%s%-40s %-10s %s%s",
			prefix,
			shared.TruncateCells(z.Name, 39),
			zoneType(z),
			"↓",
			"",
		)

		b.WriteString(rowStyle.Render(row) + "\n")
	}

	// Show records for selected zone
	b.WriteString("\n")
	if m.selectedZone != nil {
		zoneHeader := lipgloss.NewStyle().Bold(true).Foreground(shared.ColorPrimary).Render(fmt.Sprintf("Records for %s", m.selectedZone.Name))
		b.WriteString(zoneHeader + "\n")

		if len(m.recordsets) == 0 {
			b.WriteString(shared.StyleHelp.Render("  No records") + "\n")
		} else {
			recHeader := lipgloss.NewStyle().Bold(true).Foreground(shared.ColorMuted)
			b.WriteString(recHeader.Render(fmt.Sprintf("  %-35s %-10s %s", "Name", "Type", "Data")) + "\n")

			maxRecs := m.height / 2
			if maxRecs < 5 {
				maxRecs = 5
			}
			count := 0
			for _, rs := range m.recordsets {
				if count >= maxRecs {
					b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(fmt.Sprintf("  ... and %d more records", len(m.recordsets)-count)) + "\n")
					break
				}
				name := rs.Name
				if name == m.selectedZone.Name || name == m.selectedZone.Name+"." {
					name = "(zone apex)"
				}
				row := fmt.Sprintf("  %-35s %-10s %s",
					shared.TruncateCells(name, 34),
					rs.Type,
					formatRecordData(rs.Records),
				)
				b.WriteString(row + "\n")
				count++
			}
		}
	}

	return b.String()
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	if m.filter.Active() {
		return "enter confirm • esc clear"
	}
	return "↑↓ navigate zones • / filter • s/S sort • R refresh • ? help"
}

// isSelectedZone reports whether zoneID is the zone currently selected.
func (m Model) isSelectedZone(zoneID string) bool {
	return m.selectedZone != nil && m.selectedZone.ID == zoneID
}

// ForceRefresh triggers a reload.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	seq := m.refresh.Start()
	return tea.Batch(m.spinner.Tick, m.fetchZones(seq))
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.filter.SetWidth(w)
}

func (m Model) fetchZones(seq uint64) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		allPages, err := zones.List(client, nil).AllPages(ctx)
		if err != nil {
			return zonesErrMsg{seq: seq, err: err}
		}
		z, err := zones.ExtractZones(allPages)
		if err != nil {
			return zonesErrMsg{seq: seq, err: err}
		}
		return zonesLoadedMsg{seq: seq, zones: z}
	}
}

func (m Model) fetchRecordsets(zoneID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		allPages, err := recordsets.ListByZone(client, zoneID, nil).AllPages(ctx)
		if err != nil {
			return recordsetsErrMsg{zoneID: zoneID, err: err}
		}
		rs, err := recordsets.ExtractRecordSets(allPages)
		if err != nil {
			return recordsetsErrMsg{zoneID: zoneID, err: err}
		}
		return recordsetsLoadedMsg{zoneID: zoneID, recordsets: rs}
	}
}

func formatRecordData(records []string) string {
	if len(records) == 0 {
		return ""
	}
	result := records[0]
	if len(records) > 1 {
		result += fmt.Sprintf(" (+%d)", len(records)-1)
	}
	return result
}
