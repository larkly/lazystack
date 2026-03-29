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

type routerExtra struct {
	InternalNetworkName string
	InternalIPv4        string
	InternalIPv6        string
}

type routersLoadedMsg struct {
	routers      []network.Router
	networkNames map[string]string
	routerExtras map[string]routerExtra
}
type routersErrMsg struct{ err error }
type tickMsg struct{}
type sortClearMsg struct{}

// Sub-column indices matching the header layout.
var routerSortColumns = []string{"name", "status", "int_net", "int_v4", "int_v6", "ext_net", "ext_v4", "ext_v6"}

// Column widths.
const (
	colStatus  = 14
	colNetwork = 14
	colIPv4    = 16
	colIPv6    = 24
	colRoutes  = 8
)

// Model is the router list view.
type Model struct {
	client          *gophercloud.ServiceClient
	routers         []network.Router
	networkNames    map[string]string
	routerExtras    map[string]routerExtra
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
		var cursorID string
		if m.cursor >= 0 && m.cursor < len(m.routers) {
			cursorID = m.routers[m.cursor].ID
		}
		m.loading = false
		m.routers = msg.routers
		m.networkNames = msg.networkNames
		m.routerExtras = msg.routerExtras
		m.err = ""
		m.sortRouters()
		if cursorID != "" {
			for i, r := range m.routers {
				if r.ID == cursorID {
					m.cursor = i
					break
				}
			}
		}
		if m.cursor >= len(m.routers) {
			m.cursor = max(0, len(m.routers)-1)
		}
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
			var cursorID string
			if m.cursor >= 0 && m.cursor < len(m.routers) {
				cursorID = m.routers[m.cursor].ID
			}
			m.sortCol = (m.sortCol + 1) % len(routerSortColumns)
			m.sortAsc = true
			m.sortHighlight = true
			m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
			m.sortRouters()
			if cursorID != "" {
				for i, r := range m.routers {
					if r.ID == cursorID {
						m.cursor = i
						break
					}
				}
			}
			return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return sortClearMsg{}
			})
		case key.Matches(msg, shared.Keys.ReverseSort):
			var cursorID string
			if m.cursor >= 0 && m.cursor < len(m.routers) {
				cursorID = m.routers[m.cursor].ID
			}
			m.sortAsc = !m.sortAsc
			m.sortHighlight = true
			m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
			m.sortRouters()
			if cursorID != "" {
				for i, r := range m.routers {
					if r.ID == cursorID {
						m.cursor = i
						break
					}
				}
			}
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

	// Group widths: network + gap + ipv4 + gap + ipv6
	groupW := colNetwork + 1 + colIPv4 + 1 + colIPv6

	// Name column gets remaining width after fixed columns
	// prefix(2) + name + gap + status + sep + internal_group + sep + gateway_group + gap + routes
	fixedW := 2 + colStatus + 3 + groupW + 3 + groupW + 1 + colRoutes
	nameW := m.width - fixedW
	if nameW < 12 {
		nameW = 12
	}

	sep := lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(" ┃ ")
	gap := " "

	// --- Row 1: group headers ---
	blankName := strings.Repeat(" ", nameW)
	blankStatus := strings.Repeat(" ", colStatus)
	groupStyle := shared.StyleHeader
	internalLabel := groupStyle.Render(fmt.Sprintf("%-*s", groupW, "Internal"))
	gatewayLabel := groupStyle.Render(fmt.Sprintf("%-*s", groupW, "Gateway"))
	blankRoutes := strings.Repeat(" ", colRoutes)
	b.WriteString("  " + blankName + gap + blankStatus + sep + internalLabel + sep + gatewayLabel + gap + blankRoutes + "\n")

	// --- Row 2: sub-column headers ---
	// Sub-columns: 0=name, 1=status, 2=int_net, 3=int_v4, 4=int_v6, 5=ext_net, 6=ext_v4, 7=ext_v6
	type subCol struct {
		title string
		width int
	}
	subCols := []subCol{
		{"Name", nameW},
		{"Status", colStatus},
		{"Network", colNetwork},
		{"IPv4", colIPv4},
		{"IPv6", colIPv6},
		{"Network", colNetwork},
		{"IPv4", colIPv4},
		{"IPv6", colIPv6},
	}

	renderSubHeader := func(idx int, sc subCol) string {
		title := sc.title
		indicator := ""
		if idx == m.sortCol {
			if m.sortAsc {
				indicator = " ▲"
			} else {
				indicator = " ▼"
			}
		}
		text := fmt.Sprintf("%-*s", sc.width, title+indicator)
		if idx == m.sortCol && m.sortHighlight {
			return lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true).Render(text)
		}
		return shared.StyleHeader.Render(text)
	}

	nameHdr := renderSubHeader(0, subCols[0])
	statusHdr := renderSubHeader(1, subCols[1])
	intNetHdr := renderSubHeader(2, subCols[2])
	intV4Hdr := renderSubHeader(3, subCols[3])
	intV6Hdr := renderSubHeader(4, subCols[4])
	extNetHdr := renderSubHeader(5, subCols[5])
	extV4Hdr := renderSubHeader(6, subCols[6])
	extV6Hdr := renderSubHeader(7, subCols[7])
	routesHdr := shared.StyleHeader.Render(fmt.Sprintf("%-*s", colRoutes, "Routes"))

	b.WriteString("  " + nameHdr + gap + statusHdr + sep +
		intNetHdr + gap + intV4Hdr + gap + intV6Hdr + sep +
		extNetHdr + gap + extV4Hdr + gap + extV6Hdr + gap + routesHdr + "\n")

	b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(strings.Repeat("─", m.width)) + "\n")

	// --- Data rows ---
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

		extra := m.routerExtras[r.ID]

		intNet := extra.InternalNetworkName
		if intNet == "" {
			intNet = "-"
		}
		intV4 := extra.InternalIPv4
		if intV4 == "" {
			intV4 = "-"
		}
		intV6 := extra.InternalIPv6
		if intV6 == "" {
			intV6 = "-"
		}

		extNet := m.networkNames[r.ExternalGatewayNetworkID]
		if extNet == "" {
			extNet = "-"
		}
		extV4 := r.ExternalGatewayIPv4
		if extV4 == "" {
			extV4 = "-"
		}
		extV6 := r.ExternalGatewayIPv6
		if extV6 == "" {
			extV6 = "-"
		}

		routes := fmt.Sprintf("%d", len(r.Routes))

		stStyle := routerStatusStyle(r.Status)

		var rowBg color.Color
		hasBg := false
		if isCursor {
			rowBg = lipgloss.Color("#073642")
			hasBg = true
		}

		mkStyle := func(w int) lipgloss.Style {
			s := lipgloss.NewStyle().Width(w)
			if isCursor {
				s = s.Bold(true).Background(rowBg)
			}
			return s
		}

		nameS := mkStyle(nameW)
		stS := stStyle.Width(colStatus)
		if isCursor {
			stS = stS.Bold(true).Background(rowBg)
		}
		intNetS := mkStyle(colNetwork)
		intV4S := mkStyle(colIPv4)
		intV6S := mkStyle(colIPv6)
		extNetS := mkStyle(colNetwork)
		extV4S := mkStyle(colIPv4)
		extV6S := mkStyle(colIPv6)
		rtS := mkStyle(colRoutes)

		gapS := lipgloss.NewStyle()
		prefixS := lipgloss.NewStyle()
		if hasBg {
			gapS = gapS.Background(rowBg)
			prefixS = prefixS.Background(rowBg)
		}
		g := gapS.Render(" ")

		row := prefixS.Render("  ") +
			nameS.Render(truncate(name, nameW)) + g +
			stS.Render(shared.StatusIcon(r.Status)+truncate(r.Status, 12)) +
			sep +
			intNetS.Render(truncate(intNet, colNetwork)) + g +
			intV4S.Render(truncate(intV4, colIPv4)) + g +
			intV6S.Render(truncate(intV6, colIPv6)) +
			sep +
			extNetS.Render(truncate(extNet, colNetwork)) + g +
			extV4S.Render(truncate(extV4, colIPv4)) + g +
			extV6S.Render(truncate(extV6, colIPv6)) + g +
			rtS.Render(truncate(routes, colRoutes))

		if hasBg {
			rowW := lipgloss.Width(row)
			if rowW < m.width {
				row += gapS.Render(strings.Repeat(" ", m.width-rowW))
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
		ea, eb := m.routerExtras[a.ID], m.routerExtras[b.ID]
		var less bool
		switch colKey {
		case "name":
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "status":
			less = a.Status < b.Status
		case "int_net":
			less = ea.InternalNetworkName < eb.InternalNetworkName
		case "int_v4":
			less = ea.InternalIPv4 < eb.InternalIPv4
		case "int_v6":
			less = ea.InternalIPv6 < eb.InternalIPv6
		case "ext_net":
			less = m.networkNames[a.ExternalGatewayNetworkID] < m.networkNames[b.ExternalGatewayNetworkID]
		case "ext_v4":
			less = a.ExternalGatewayIPv4 < b.ExternalGatewayIPv4
		case "ext_v6":
			less = a.ExternalGatewayIPv6 < b.ExternalGatewayIPv6
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
	h := m.height - 6 // extra line for two-row header
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
		ctx := context.Background()

		rlist, err := network.ListRouters(ctx, client)
		if err != nil {
			return routersErrMsg{err: err}
		}

		nets, err := network.ListNetworks(ctx, client)
		if err != nil {
			return routersErrMsg{err: err}
		}
		networkNames := make(map[string]string, len(nets))
		for _, n := range nets {
			networkNames[n.ID] = n.Name
		}

		subs, err := network.ListSubnets(ctx, client)
		if err != nil {
			return routersErrMsg{err: err}
		}
		subnetToNetwork := make(map[string]string, len(subs))
		for _, s := range subs {
			subnetToNetwork[s.ID] = s.NetworkID
		}

		extras := make(map[string]routerExtra, len(rlist))
		for _, r := range rlist {
			ifaces, err := network.ListRouterInterfaces(ctx, client, r.ID)
			if err != nil {
				continue
			}
			var ex routerExtra
			for _, iface := range ifaces {
				if ex.InternalNetworkName == "" {
					if netID := subnetToNetwork[iface.SubnetID]; netID != "" {
						ex.InternalNetworkName = networkNames[netID]
					}
				}
				if strings.Contains(iface.IPAddress, ":") {
					if ex.InternalIPv6 == "" {
						ex.InternalIPv6 = iface.IPAddress
					}
				} else {
					if ex.InternalIPv4 == "" {
						ex.InternalIPv4 = iface.IPAddress
					}
				}
			}
			extras[r.ID] = ex
		}

		return routersLoadedMsg{
			routers:      rlist,
			networkNames: networkNames,
			routerExtras: extras,
		}
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
