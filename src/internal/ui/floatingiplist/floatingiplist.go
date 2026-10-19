package floatingiplist

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/copypicker"
	"github.com/larkly/lazystack/internal/ui/filterbar"
)

type fipsLoadedMsg struct {
	seq  uint64
	fips []network.FloatingIP
}
type fipsErrMsg struct {
	seq uint64
	err error
}
type sortClearMsg struct{}

var fipSortColumns = []string{"floatingip", "status", "fixedip", "portid"}

// Model is the floating IP list view.
type Model struct {
	client          *gophercloud.ServiceClient
	allFIPs         []network.FloatingIP // every floating IP, as last fetched
	fips            []network.FloatingIP // the listed ones: filtered and sorted
	filter          filterbar.Model
	cursor          int
	width           int
	height          int
	loading         bool
	refresh         shared.RefreshGate
	spinner         spinner.Model
	err             error
	scrollOff       int
	refreshInterval time.Duration
	order           shared.SortState
}

// New creates a floating IP list model.
func New(client *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:          client,
		loading:         true,
		spinner:         s,
		refreshInterval: refreshInterval,
		order:           shared.NewSortState(),
		filter:          filterbar.New(),
	}
}

// IsFiltering reports whether the "/" filter input has focus.
func (m Model) IsFiltering() bool { return m.filter.Active() }

// FilterValue is the current filter text.
func (m Model) FilterValue() string { return m.filter.Value() }

// Escape clears the committed filter and reports whether there was one;
// when there was not, esc goes on to mean back-navigation in the root model.
func (m *Model) Escape() (handled bool) {
	if !m.filter.Clear() {
		return false
	}
	m.applyFilter()
	return true
}

// Init starts the initial fetch.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[floatingiplist] Init()")
	return tea.Batch(m.spinner.Tick, m.fetchFIPs(m.refresh.Seq()))
}

// SelectedFIP returns the floating IP under the cursor.
func (m Model) SelectedFIP() *network.FloatingIP {
	if m.cursor >= 0 && m.cursor < len(m.fips) {
		f := m.fips[m.cursor]
		return &f
	}
	return nil
}

// CopyEntries returns the title and copyable fields for the selected floating IP.
func (m Model) CopyEntries() (string, []copypicker.Entry) {
	f := m.SelectedFIP()
	if f == nil {
		return "", nil
	}
	b := copypicker.Builder{}
	b.Add("ID", f.ID).
		Add("Floating IP", f.FloatingIP).
		Add("Fixed IP", f.FixedIP).
		Add("Port ID", f.PortID)
	return "Copy — floating IP " + f.FloatingIP, b.Entries()
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case fipsLoadedMsg:
		if !m.refresh.Accept(msg.seq) {
			return m, nil
		}
		shared.Debugf("[floatingiplist] loaded %d floating IPs", len(msg.fips))
		m.loading = false
		m.allFIPs = msg.fips
		m.err = nil
		m.applyFilter()
		return m, nil

	case fipsErrMsg:
		if !m.refresh.Accept(msg.seq) {
			return m, nil
		}
		shared.Debugf("[floatingiplist] error: %v", msg.err)
		m.loading = false
		m.err = msg.err
		return m, nil

	case shared.TickMsg:
		if m.loading || m.refresh.Busy() {
			shared.Debugf("[floatingiplist] tick skipped (fetch in flight)")
			return m, nil
		}
		shared.Debugf("[floatingiplist] tick fetching")
		seq := m.refresh.Start()
		return m, m.fetchFIPs(seq)

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

	case tea.KeyMsg:
		if m.filter.Active() {
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			m.applyFilter()
			return m, cmd
		}
		switch {
		case key.Matches(msg, shared.Keys.Filter):
			return m, m.filter.Start()
		case key.Matches(msg, shared.Keys.Back):
			m.Escape()
		case key.Matches(msg, shared.Keys.Sort):
			m.order = shared.NextSort(m.order, len(fipSortColumns))
			return m.resort()
		case key.Matches(msg, shared.Keys.ReverseSort):
			m.order = shared.ReverseSort(m.order)
			return m.resort()
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.fips)-1 {
				m.cursor++
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.PageDown):
			m.cursor += m.tableHeight()
			if m.cursor >= len(m.fips) {
				m.cursor = len(m.fips) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.ensureVisible()
		case key.Matches(msg, shared.Keys.PageUp):
			m.cursor -= m.tableHeight()
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.ensureVisible()
		}
	}
	return m, nil
}

// cursorID is the ID of the floating IP under the cursor, or "".
func (m Model) cursorID() string {
	if f := m.SelectedFIP(); f != nil {
		return f.ID
	}
	return ""
}

// rowIDs lists the IDs of the listed floating IPs in display order.
func (m Model) rowIDs() []string {
	ids := make([]string, len(m.fips))
	for i, f := range m.fips {
		ids[i] = f.ID
	}
	return ids
}

// applyFilter rebuilds the listed floating IPs from every floating IP under
// the current filter and sort, keeping the cursor on the same one while it
// is still listed and the viewport where it was.
func (m *Model) applyFilter() {
	cursorID := m.cursorID()
	m.fips = m.fips[:0:0]
	for _, f := range m.allFIPs {
		if m.filter.Match(f.FloatingIP, f.FixedIP, f.Status, f.PortID, f.ID) {
			m.fips = append(m.fips, f)
		}
	}
	m.sortFIPs()
	if i := shared.RestoreCursor(m.rowIDs(), cursorID); i >= 0 {
		m.cursor = i
	}
	m.cursor = shared.ClampCursor(m.cursor, len(m.fips))
	m.scrollOff = min(m.scrollOff, max(0, len(m.fips)-m.tableHeight()))
	m.ensureVisible()
}

// resort re-sorts the list after a sort change, keeping the cursor on the
// same floating IP, and highlights the sort column for a moment.
func (m Model) resort() (Model, tea.Cmd) {
	id := m.cursorID()
	m.sortFIPs()
	if i := shared.RestoreCursor(m.rowIDs(), id); i >= 0 {
		m.cursor = i
	}
	m.ensureVisible()
	return m, shared.SortHighlightCmd(sortClearMsg{})
}

func (m *Model) ensureVisible() {
	th := m.tableHeight()
	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+th {
		m.scrollOff = m.cursor - th + 1
	}
}

// tableHeight is the number of floating IP rows that fit: the app gives the view
// the full terminal height and adds a tab bar above it and a status bar below
// it, and the view itself uses title, error/blank line, header and
// separator, plus the filter line while a filter is shown.
func (m Model) tableHeight() int {
	h := m.height - 6
	if m.filter.Visible() {
		h--
	}
	return max(1, h)
}

// View renders the floating IP list.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Floating IPs")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.fips))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n")
	b.WriteString(shared.LoadErrorLine(m.err, m.width) + "\n")
	if bar := m.filter.View(); bar != "" {
		b.WriteString(bar + "\n")
	}

	if len(m.fips) == 0 && !m.loading {
		msg := "  No floating IPs found."
		if m.filter.Value() != "" {
			msg = "  No floating IPs match the filter."
		}
		b.WriteString(shared.StyleHelp.Render(msg) + "\n")
		return b.String()
	}

	headerTitles := []struct {
		title string
		width int
	}{
		{"Floating IP", 16},
		{"Status", 14},
		{"Fixed IP", 16},
		{"Port ID", 36},
	}
	var headerParts []string
	for i, h := range headerTitles {
		title := h.title
		indicator := m.order.Indicator(i)
		if i == m.order.Col && m.order.Highlight {
			headerParts = append(headerParts, lipgloss.NewStyle().
				Foreground(shared.ColorHighlight).
				Bold(true).
				Render(fmt.Sprintf("%-*s", h.width, title+indicator)))
		} else {
			headerParts = append(headerParts, shared.StyleHeader.Render(fmt.Sprintf("%-*s", h.width, title+indicator)))
		}
	}
	b.WriteString("  " + strings.Join(headerParts, " ") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(strings.Repeat("─", m.width)) + "\n")

	th := m.tableHeight()
	end := m.scrollOff + th
	if end > len(m.fips) {
		end = len(m.fips)
	}

	for i := m.scrollOff; i < end; i++ {
		f := m.fips[i]
		cursor := "  "
		if i == m.cursor {
			cursor = "▸ "
		}

		portID := f.PortID
		if len(portID) > 36 {
			portID = portID[:35] + "…"
		}
		fixedIP := f.FixedIP
		if fixedIP == "" {
			fixedIP = "-"
		}

		statusStyle := fipStatusStyle(f.Status)
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.cursor {
			style = style.Background(lipgloss.Color("#073642")).Bold(true)
			statusStyle = statusStyle.Background(lipgloss.Color("#073642")).Bold(true)
		}

		line := fmt.Sprintf("%-16s ", f.FloatingIP)
		statusStr := statusStyle.Width(14).Render(shared.StatusIcon(f.Status) + f.Status)
		rest := fmt.Sprintf(" %-16s %-36s", fixedIP, portID)

		b.WriteString(cursor + style.Render(line) + statusStr + style.Render(rest) + "\n")
	}

	return b.String()
}

func (m *Model) sortFIPs() {
	if len(m.fips) == 0 {
		return
	}
	colKey := fipSortColumns[m.order.Col]
	asc := m.order.Asc
	sort.SliceStable(m.fips, func(i, j int) bool {
		a, b := m.fips[i], m.fips[j]
		if !asc {
			a, b = b, a
		}
		var less bool
		switch colKey {
		case "floatingip":
			less = a.FloatingIP < b.FloatingIP
		case "status":
			less = a.Status < b.Status
		case "fixedip":
			less = a.FixedIP < b.FixedIP
		case "portid":
			less = a.PortID < b.PortID
		default:
			less = false
		}
		return less
	})
}

func fipStatusStyle(status string) lipgloss.Style {
	var fg = shared.ColorFg
	switch status {
	case "ACTIVE":
		fg = shared.ColorSuccess
	case "DOWN":
		fg = shared.ColorMuted
	case "ERROR":
		fg = shared.ColorError
	}
	return lipgloss.NewStyle().Foreground(fg)
}

func (m Model) fetchFIPs(seq uint64) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		shared.Debugf("[floatingiplist] fetch start")
		fips, err := network.ListFloatingIPs(ctx, client)
		if err != nil {
			shared.Debugf("[floatingiplist] fetch error: %v", err)
			return fipsErrMsg{seq: seq, err: err}
		}
		shared.Debugf("[floatingiplist] fetch done, count=%d", len(fips))
		return fipsLoadedMsg{seq: seq, fips: fips}
	}
}

// ForceRefresh triggers a manual reload of the floating IP list.
func (m *Model) ForceRefresh() tea.Cmd {
	shared.Debugf("[floatingiplist] ForceRefresh()")
	m.loading = true
	seq := m.refresh.Start()
	return tea.Batch(m.spinner.Tick, m.fetchFIPs(seq))
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.filter.SetWidth(w)
}

// Hints returns key hints.
func (m Model) Hints() string {
	if m.filter.Active() {
		return "enter confirm • esc clear"
	}
	return "↑↓ navigate • / filter • s/S sort • ^n allocate • ^t disassociate • ^d release • R refresh • ? help"
}
