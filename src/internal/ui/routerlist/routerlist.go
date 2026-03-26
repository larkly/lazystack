package routerlist

import (
	"context"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type routersLoadedMsg struct{ routers []network.Router }
type routersErrMsg struct{ err error }
type tickMsg struct{}
type sortClearMsg struct{}

var routerSortColumns = []string{"name", "status", "gateway"}

// Model is the router list view.
type Model struct {
	client          *gophercloud.ServiceClient
	routers         []network.Router
	cursor          int
	scrollOff       int
	width           int
	height          int
	loading         bool
	spinner         spinner.Model
	err             string
	refreshInterval time.Duration
	sortCol         int
	sortAsc         bool
	sortHighlight   bool
	sortClearAt     time.Time
}

// New creates a router list model.
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
	return tea.Batch(m.spinner.Tick, m.fetchRouters(), m.tickCmd())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case routersLoadedMsg:
		m.loading = false
		m.routers = msg.routers
		m.err = ""
		m.sortRouters()
		return m, nil

	case routersErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.fetchRouters(), m.tickCmd())

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
			m.sortCol = (m.sortCol + 1) % len(routerSortColumns)
			m.sortAsc = true
			m.sortHighlight = true
			m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
			m.sortRouters()
			return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return sortClearMsg{}
			})
		case key.Matches(msg, shared.Keys.ReverseSort):
			m.sortAsc = !m.sortAsc
			m.sortHighlight = true
			m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
			m.sortRouters()
			return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return sortClearMsg{}
			})
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.routers)-1 {
				m.cursor++
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.PageDown):
			m.cursor += m.tableHeight()
			if m.cursor >= len(m.routers) {
				m.cursor = len(m.routers) - 1
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

// View renders the router list.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Routers")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.routers))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.routers) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No routers found.") + "\n")
		return b.String()
	}

	// Name column gets remaining width after fixed columns
	fixedW := 12 + 20 + 8 + 3 + 2 // status + gateway + routes + gaps + prefix
	nameW := m.width - fixedW
	if nameW < 20 {
		nameW = 20
	}

	headerTitles := []struct {
		title string
		width int
	}{
		{"Name", nameW},
		{"Status", 12},
		{"External Gateway", 20},
		{"Routes", 8},
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
	if end > len(m.routers) {
		end = len(m.routers)
	}

	for i := m.scrollOff; i < end; i++ {
		r := m.routers[i]
		isCursor := i == m.cursor

		name := r.Name
		if name == "" && len(r.ID) > 8 {
			name = r.ID[:8] + "..."
		}

		gateway := r.ExternalGatewayIP
		if gateway == "" {
			gateway = "none"
		}

		routes := fmt.Sprintf("%d", len(r.Routes))

		statusStyle := routerStatusStyle(r.Status)

		var rowBg color.Color
		hasBg := false
		if isCursor {
			rowBg = lipgloss.Color("#073642")
			hasBg = true
		}

		nameStyle := lipgloss.NewStyle().Width(nameW)
		stStyle := statusStyle.Width(12)
		gwStyle := lipgloss.NewStyle().Width(20)
		rtStyle := lipgloss.NewStyle().Width(8)

		if isCursor {
			nameStyle = nameStyle.Bold(true).Background(rowBg)
			stStyle = stStyle.Bold(true).Background(rowBg)
			gwStyle = gwStyle.Bold(true).Background(rowBg)
			rtStyle = rtStyle.Bold(true).Background(rowBg)
		}

		parts := []string{
			nameStyle.Render(truncate(name, nameW)),
			stStyle.Render(truncate(r.Status, 12)),
			gwStyle.Render(truncate(gateway, 20)),
			rtStyle.Render(truncate(routes, 8)),
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

func routerStatusStyle(status string) lipgloss.Style {
	var fg color.Color = shared.ColorFg
	switch status {
	case "ACTIVE":
		fg = shared.ColorSuccess
	case "BUILD":
		fg = shared.ColorWarning
	case "ERROR":
		fg = shared.ColorError
	default:
		fg = shared.ColorMuted
	}
	return lipgloss.NewStyle().Foreground(fg)
}

func (m *Model) sortRouters() {
	if len(m.routers) == 0 {
		return
	}
	colKey := routerSortColumns[m.sortCol]
	asc := m.sortAsc
	sort.SliceStable(m.routers, func(i, j int) bool {
		a, b := m.routers[i], m.routers[j]
		var less bool
		switch colKey {
		case "name":
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "status":
			less = a.Status < b.Status
		case "gateway":
			less = a.ExternalGatewayIP < b.ExternalGatewayIP
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

// SelectedRouter returns the router under the cursor.
func (m Model) SelectedRouter() *network.Router {
	if m.cursor >= 0 && m.cursor < len(m.routers) {
		r := m.routers[m.cursor]
		return &r
	}
	return nil
}

func (m Model) fetchRouters() tea.Cmd {
	client := m.client
	if client == nil {
		return func() tea.Msg {
			return routersErrMsg{err: fmt.Errorf("network service not available")}
		}
	}
	return func() tea.Msg {
		routers, err := network.ListRouters(context.Background(), client)
		if err != nil {
			return routersErrMsg{err: err}
		}
		return routersLoadedMsg{routers: routers}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// ForceRefresh triggers a manual reload of the router list.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchRouters())
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "↑↓ navigate • enter detail • ^n create • ^d delete • R refresh • 1-5/←→ switch tab • ? help"
}
