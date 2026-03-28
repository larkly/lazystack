package lblist

import (
	"context"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type lbsLoadedMsg struct{ lbs []loadbalancer.LoadBalancer }
type lbsErrMsg struct{ err error }
type tickMsg struct{}
type sortClearMsg struct{}

var lbSortColumns = []string{"name", "vipaddress", "provstatus", "operstatus"}

// Model is the load balancer list view.
type Model struct {
	client          *gophercloud.ServiceClient
	lbs             []loadbalancer.LoadBalancer
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

// New creates a load balancer list model.
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
	return tea.Batch(m.spinner.Tick, m.fetchLBs(), m.tickCmd())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case lbsLoadedMsg:
		m.loading = false
		m.lbs = msg.lbs
		m.err = ""
		m.sortLBs()
		return m, nil

	case lbsErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.fetchLBs(), m.tickCmd())

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
			m.sortCol = (m.sortCol + 1) % len(lbSortColumns)
			m.sortAsc = true
			m.sortHighlight = true
			m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
			m.sortLBs()
			return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return sortClearMsg{}
			})
		case key.Matches(msg, shared.Keys.ReverseSort):
			m.sortAsc = !m.sortAsc
			m.sortHighlight = true
			m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
			m.sortLBs()
			return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return sortClearMsg{}
			})
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.lbs)-1 {
				m.cursor++
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.PageDown):
			m.cursor += m.tableHeight()
			if m.cursor >= len(m.lbs) {
				m.cursor = len(m.lbs) - 1
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

// View renders the load balancer list.
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

	// Name column gets remaining width after fixed columns
	fixedW := 18 + 18 + 16 + 3 + 2 // vip + prov + oper + gaps + prefix
	nameW := m.width - fixedW
	if nameW < 20 {
		nameW = 20
	}

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
	if end > len(m.lbs) {
		end = len(m.lbs)
	}

	for i := m.scrollOff; i < end; i++ {
		lb := m.lbs[i]
		isCursor := i == m.cursor

		name := lb.Name
		if name == "" && len(lb.ID) > 8 {
			name = lb.ID[:8] + "..."
		}

		provStyle := lbStatusStyle(lb.ProvisioningStatus)
		operStyle := lbOperStatusStyle(lb.OperatingStatus)

		var rowBg color.Color
		hasBg := false
		if isCursor {
			rowBg = lipgloss.Color("#073642")
			hasBg = true
		}

		nameStyle := lipgloss.NewStyle().Width(nameW)
		vipStyle := lipgloss.NewStyle().Width(18)
		psStyle := provStyle.Width(18)
		osStyle := operStyle.Width(16)

		if isCursor {
			nameStyle = nameStyle.Bold(true).Background(rowBg)
			vipStyle = vipStyle.Bold(true).Background(rowBg)
			psStyle = psStyle.Bold(true).Background(rowBg)
			osStyle = osStyle.Bold(true).Background(rowBg)
		}

		parts := []string{
			nameStyle.Render(truncate(name, nameW)),
			vipStyle.Render(truncate(lb.VipAddress, 18)),
			psStyle.Render(shared.StatusIcon(lb.ProvisioningStatus) + truncate(lb.ProvisioningStatus, 16)),
			osStyle.Render(shared.StatusIcon(lb.OperatingStatus) + truncate(lb.OperatingStatus, 14)),
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
			if rowW < m.width {
				row += gapStyle.Render(strings.Repeat(" ", m.width-rowW))
			}
		}

		b.WriteString(row + "\n")
	}

	return b.String()
}

func truncate(s string, w int) string {
	if len(s) > w && w > 3 {
		return s[:w-3] + "..."
	}
	if len(s) > w {
		return s[:w]
	}
	return s
}

func lbStatusStyle(status string) lipgloss.Style {
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

func lbOperStatusStyle(status string) lipgloss.Style {
	var fg color.Color = shared.ColorFg
	switch status {
	case "ONLINE":
		fg = shared.ColorSuccess
	case "OFFLINE":
		fg = shared.ColorError
	}
	return lipgloss.NewStyle().Foreground(fg)
}

func (m *Model) sortLBs() {
	if len(m.lbs) == 0 {
		return
	}
	colKey := lbSortColumns[m.sortCol]
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

// SelectedLB returns the load balancer under the cursor.
func (m Model) SelectedLB() *loadbalancer.LoadBalancer {
	if m.cursor >= 0 && m.cursor < len(m.lbs) {
		lb := m.lbs[m.cursor]
		return &lb
	}
	return nil
}

func (m Model) fetchLBs() tea.Cmd {
	client := m.client
	if client == nil {
		return func() tea.Msg {
			return lbsErrMsg{err: fmt.Errorf("load balancer service not available")}
		}
	}
	return func() tea.Msg {
		lbs, err := loadbalancer.ListLoadBalancers(context.Background(), client)
		if err != nil {
			return lbsErrMsg{err: err}
		}
		return lbsLoadedMsg{lbs: lbs}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// ForceRefresh triggers a manual reload of the load balancer list.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchLBs())
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "↑↓ navigate • enter detail • ^d delete • R refresh • 1-6/←→ switch tab • ? help"
}
