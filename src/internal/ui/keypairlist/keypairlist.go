package keypairlist

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
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/copypicker"
	"github.com/larkly/lazystack/internal/ui/filterbar"
)

type keypairsLoadedMsg struct {
	seq      uint64
	keypairs []compute.KeyPair
}
type keypairsErrMsg struct {
	seq uint64
	err error
}
type sortClearMsg struct{}

var kpSortColumns = []string{"name", "type"}

// Model is the keypair list view.
type Model struct {
	client          *gophercloud.ServiceClient
	allPairs        []compute.KeyPair // every key pair, as last fetched
	pairs           []compute.KeyPair // the listed ones: filtered and sorted
	filter          filterbar.Model
	cursor          int
	scrollOff       int
	width           int
	height          int
	loading         bool
	refresh         shared.RefreshGate
	spinner         spinner.Model
	err             error
	order           shared.SortState
	refreshInterval time.Duration
	highlight       string // key pair name to put the cursor on once listed
}

// New creates a keypair list model.
func New(client *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:          client,
		loading:         true,
		spinner:         s,
		order:           shared.NewSortState(),
		refreshInterval: refreshInterval,
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
	shared.Debugf("[keypairlist] Init()")
	return tea.Batch(m.spinner.Tick, m.fetchKeypairs(m.refresh.Seq()))
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case keypairsLoadedMsg:
		if !m.refresh.Accept(msg.seq) {
			return m, nil
		}
		shared.Debugf("[keypairlist] loaded %d keypairs", len(msg.keypairs))
		m.loading = false
		m.allPairs = msg.keypairs
		m.err = nil
		m.applyFilter()
		m.applyHighlight()
		return m, nil

	case keypairsErrMsg:
		if !m.refresh.Accept(msg.seq) {
			return m, nil
		}
		shared.Debugf("[keypairlist] error: %v", msg.err)
		m.loading = false
		m.err = msg.err
		return m, nil

	case shared.TickMsg:
		if m.loading || m.refresh.Busy() {
			shared.Debugf("[keypairlist] tick skipped (fetch in flight)")
			return m, nil
		}
		shared.Debugf("[keypairlist] tick fetching")
		seq := m.refresh.Start()
		return m, m.fetchKeypairs(seq)

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
			m.order = shared.NextSort(m.order, len(kpSortColumns))
			return m.resort()
		case key.Matches(msg, shared.Keys.ReverseSort):
			m.order = shared.ReverseSort(m.order)
			return m.resort()
		case key.Matches(msg, shared.Keys.Up):
			m.cursor--
			m.ensureVisible()
		case key.Matches(msg, shared.Keys.Down):
			m.cursor++
			m.ensureVisible()
		case key.Matches(msg, shared.Keys.PageDown):
			m.cursor += m.tableHeight()
			m.ensureVisible()
		case key.Matches(msg, shared.Keys.PageUp):
			m.cursor -= m.tableHeight()
			m.ensureVisible()
		}
	}
	return m, nil
}

// View renders the keypair list.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Key Pairs")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.pairs))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n")
	b.WriteString(shared.LoadErrorLine(m.err, m.width) + "\n")
	if bar := m.filter.View(); bar != "" {
		b.WriteString(bar + "\n")
	}

	if len(m.pairs) == 0 && !m.loading {
		msg := "  No key pairs found."
		if m.filter.Value() != "" {
			msg = "  No key pairs match the filter."
		}
		b.WriteString(shared.StyleHelp.Render(msg) + "\n")
		return b.String()
	}

	headerTitles := []struct {
		title string
		width int
	}{
		{"Name", 30},
		{"Type", 10},
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

	end := min(len(m.pairs), m.scrollOff+m.tableHeight())
	for i := m.scrollOff; i < end; i++ {
		kp := m.pairs[i]
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.cursor {
			cursor = "▸ "
			style = style.Background(lipgloss.Color("#073642")).Bold(true)
		}

		line := fmt.Sprintf("%-30s %-10s", kp.Name, kp.Type)
		b.WriteString(cursor + style.Render(line) + "\n")
	}

	return b.String()
}

// cursorName is the name (a key pair's ID) of the key pair under the
// cursor, or "".
func (m Model) cursorName() string {
	if kp := m.SelectedKeyPair(); kp != nil {
		return kp.Name
	}
	return ""
}

// rowNames lists the names of the listed key pairs in display order.
func (m Model) rowNames() []string {
	names := make([]string, len(m.pairs))
	for i, kp := range m.pairs {
		names[i] = kp.Name
	}
	return names
}

// applyFilter rebuilds the listed key pairs from every key pair under the
// current filter and sort, keeping the cursor on the same key pair while it
// is still listed.
func (m *Model) applyFilter() {
	name := m.cursorName()
	m.pairs = m.pairs[:0:0]
	for _, kp := range m.allPairs {
		if m.filter.Match(kp.Name, kp.Type) {
			m.pairs = append(m.pairs, kp)
		}
	}
	m.sortPairs()
	if i := shared.RestoreCursor(m.rowNames(), name); i >= 0 {
		m.cursor = i
	}
	m.ensureVisible()
}

// resort re-sorts the list after a sort change, keeping the cursor on the
// same key pair, and highlights the sort column for a moment.
func (m Model) resort() (Model, tea.Cmd) {
	name := m.cursorName()
	m.sortPairs()
	if i := shared.RestoreCursor(m.rowNames(), name); i >= 0 {
		m.cursor = i
	}
	m.ensureVisible()
	return m, shared.SortHighlightCmd(sortClearMsg{})
}

// SetHighlight puts the cursor on the key pair with the given name as soon
// as it is listed; a key pair just created may only appear after a refresh.
func (m *Model) SetHighlight(name string) {
	m.highlight = name
	m.applyHighlight()
}

func (m *Model) applyHighlight() {
	if m.highlight == "" {
		return
	}
	// A committed filter that hides the key pair being highlighted gives way.
	if m.filter.Value() != "" && shared.RestoreCursor(m.rowNames(), m.highlight) < 0 {
		for _, kp := range m.allPairs {
			if kp.Name == m.highlight {
				m.filter.Clear()
				m.applyFilter()
				break
			}
		}
	}
	for i, kp := range m.pairs {
		if kp.Name == m.highlight {
			m.cursor = i
			m.ensureVisible()
			m.highlight = ""
			return
		}
	}
}

// ensureVisible clamps the cursor to the list and scrolls so the cursor row
// is visible without leaving blank rows below the last keypair.
func (m *Model) ensureVisible() {
	m.cursor = max(0, min(m.cursor, len(m.pairs)-1))
	th := m.tableHeight()
	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+th {
		m.scrollOff = m.cursor - th + 1
	}
	m.scrollOff = max(0, min(m.scrollOff, len(m.pairs)-th))
}

// tableHeight is the number of keypair rows that fit: the app gives the view
// the terminal height minus the tab bar and status bar, and the view itself
// uses title, error/blank line, header and separator, plus the filter line
// while a filter is shown.
func (m Model) tableHeight() int {
	h := m.height - 6
	if m.filter.Visible() {
		h--
	}
	return max(1, h)
}

func (m *Model) sortPairs() {
	if len(m.pairs) == 0 {
		return
	}
	colKey := kpSortColumns[m.order.Col]
	asc := m.order.Asc
	sort.SliceStable(m.pairs, func(i, j int) bool {
		a, b := m.pairs[i], m.pairs[j]
		if !asc {
			a, b = b, a
		}
		var less bool
		switch colKey {
		case "name":
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "type":
			less = strings.ToLower(a.Type) < strings.ToLower(b.Type)
		default:
			less = false
		}
		return less
	})
}

// SelectedKeyPair returns the keypair under the cursor.
func (m Model) SelectedKeyPair() *compute.KeyPair {
	if m.cursor >= 0 && m.cursor < len(m.pairs) {
		kp := m.pairs[m.cursor]
		return &kp
	}
	return nil
}

// CopyEntries returns the title and copyable fields for the selected keypair.
func (m Model) CopyEntries() (string, []copypicker.Entry) {
	kp := m.SelectedKeyPair()
	if kp == nil {
		return "", nil
	}
	b := copypicker.Builder{}
	b.Add("Name", kp.Name)
	return "Copy — keypair " + kp.Name, b.Entries()
}

func (m Model) fetchKeypairs(seq uint64) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		shared.Debugf("[keypairlist] fetch start")
		kps, err := compute.ListKeyPairs(ctx, client)
		if err != nil {
			shared.Debugf("[keypairlist] fetch error: %v", err)
			return keypairsErrMsg{seq: seq, err: err}
		}
		shared.Debugf("[keypairlist] fetch done, count=%d", len(kps))
		return keypairsLoadedMsg{seq: seq, keypairs: kps}
	}
}

// ForceRefresh triggers a manual reload of the keypair list.
func (m *Model) ForceRefresh() tea.Cmd {
	shared.Debugf("[keypairlist] ForceRefresh()")
	m.loading = true
	seq := m.refresh.Start()
	return tea.Batch(m.spinner.Tick, m.fetchKeypairs(seq))
}

// SetSize updates dimensions and keeps the cursor row visible.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.filter.SetWidth(w)
	m.ensureVisible()
}

// Hints returns key hints.
func (m Model) Hints() string {
	if m.filter.Active() {
		return "enter confirm • esc clear"
	}
	return "↑↓ navigate • enter detail • / filter • s/S sort • ^n create • ^d delete • R refresh • 1-9, 0/←→ switch tab • ? help"
}
