package floatingiplist

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/network"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type fipsLoadedMsg struct{ fips []network.FloatingIP }
type fipsErrMsg struct{ err error }
type tickMsg struct{}
type sortClearMsg struct{}

var fipSortColumns = []string{"floatingip", "status", "fixedip", "portid"}

// Model is the floating IP list view.
type Model struct {
	client          *gophercloud.ServiceClient
	fips            []network.FloatingIP
	cursor          int
	width           int
	height          int
	loading         bool
	spinner         spinner.Model
	err             string
	scrollOff       int
	refreshInterval time.Duration
	sortCol         int
	sortAsc         bool
	sortHighlight   bool
	sortClearAt     time.Time
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
		sortAsc:         true,
	}
}

// Init starts the initial fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchFIPs(), m.tickCmd())
}

// SelectedFIP returns the floating IP under the cursor.
func (m Model) SelectedFIP() *network.FloatingIP {
	if m.cursor >= 0 && m.cursor < len(m.fips) {
		f := m.fips[m.cursor]
		return &f
	}
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case fipsLoadedMsg:
		m.loading = false
		m.fips = msg.fips
		m.err = ""
		m.sortFIPs()
		return m, nil

	case fipsErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.fetchFIPs(), m.tickCmd())

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

	case sortClearMsg:
		m.sortHighlight = false
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Sort):
			m.sortCol = (m.sortCol + 1) % len(fipSortColumns)
			m.sortAsc = true
			m.sortHighlight = true
			m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
			m.sortFIPs()
			return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return sortClearMsg{}
			})
		case key.Matches(msg, shared.Keys.ReverseSort):
			m.sortAsc = !m.sortAsc
			m.sortHighlight = true
			m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
			m.sortFIPs()
			return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return sortClearMsg{}
			})
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

func (m *Model) ensureVisible() {
	th := m.tableHeight()
	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+th {
		m.scrollOff = m.cursor - th + 1
	}
}

func (m Model) tableHeight() int {
	h := m.height - 5
	if h < 1 {
		h = 1
	}
	return h
}

// View renders the floating IP list.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Floating IPs")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.fips))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.fips) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No floating IPs found.") + "\n")
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
		indicator := ""
		if i == m.sortCol {
			if m.sortAsc {
				indicator = " ▲"
			} else {
				indicator = " ▼"
			}
		}
		if i == m.sortCol && m.sortHighlight {
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
	colKey := fipSortColumns[m.sortCol]
	asc := m.sortAsc
	sort.SliceStable(m.fips, func(i, j int) bool {
		a, b := m.fips[i], m.fips[j]
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
		if !asc {
			return !less
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

func (m Model) fetchFIPs() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		fips, err := network.ListFloatingIPs(context.Background(), client)
		if err != nil {
			return fipsErrMsg{err: err}
		}
		return fipsLoadedMsg{fips: fips}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// ForceRefresh triggers a manual reload of the floating IP list.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchFIPs())
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "↑↓ navigate • ^n allocate • ^t disassociate • ^d release • R refresh • ? help"
}
