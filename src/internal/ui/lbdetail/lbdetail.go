package lbdetail

import (
	"context"
	"fmt"
	"image/color"
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

type lbDetailLoadedMsg struct {
	lb        *loadbalancer.LoadBalancer
	listeners []loadbalancer.Listener
	pools     []loadbalancer.Pool
	members   map[string][]loadbalancer.Member
}

type lbDetailErrMsg struct {
	err error
}

type detailTickMsg struct{}

// Model is the load balancer detail view.
type Model struct {
	client          *gophercloud.ServiceClient
	lbID            string
	lb              *loadbalancer.LoadBalancer
	listeners       []loadbalancer.Listener
	pools           []loadbalancer.Pool
	members         map[string][]loadbalancer.Member
	loading         bool
	spinner         spinner.Model
	width           int
	height          int
	scroll          int
	err             string
	refreshInterval time.Duration
}

// New creates a load balancer detail model.
func New(client *gophercloud.ServiceClient, lbID string, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:          client,
		lbID:            lbID,
		loading:         true,
		spinner:         s,
		members:         make(map[string][]loadbalancer.Member),
		refreshInterval: refreshInterval,
	}
}

// Init fetches the load balancer details.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchDetail(), m.tickCmd())
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

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case lbDetailLoadedMsg:
		m.loading = false
		m.lb = msg.lb
		m.listeners = msg.listeners
		m.pools = msg.pools
		m.members = msg.members
		m.err = ""
		return m, nil

	case lbDetailErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case detailTickMsg:
		return m, tea.Batch(m.fetchDetail(), m.tickCmd())

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
		switch {
		case key.Matches(msg, shared.Keys.Back):
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "lblist"}
			}
		case key.Matches(msg, shared.Keys.Up):
			if m.scroll > 0 {
				m.scroll--
			}
		case key.Matches(msg, shared.Keys.Down):
			m.scroll++
		case key.Matches(msg, shared.Keys.PageDown):
			m.scroll += m.height - 5
		case key.Matches(msg, shared.Keys.PageUp):
			m.scroll -= m.height - 5
			if m.scroll < 0 {
				m.scroll = 0
			}
		}
	}
	return m, nil
}

// View renders the load balancer detail.
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

	lb := m.lb

	lines := make([]string, 0, 32)

	// Header
	headerLine := fmt.Sprintf("  %s",
		lipgloss.NewStyle().Bold(true).Foreground(shared.ColorPrimary).
			Render(fmt.Sprintf("=== Load Balancer: %s ===", displayName(lb.Name, lb.ID))))
	lines = append(lines, headerLine, "")

	// Properties
	props := []struct {
		label string
		value string
		style func(string) lipgloss.Style
	}{
		{"ID", lb.ID, nil},
		{"VIP Address", lb.VipAddress, nil},
		{"Prov Status", lb.ProvisioningStatus, provStatusStyleFn},
		{"Oper Status", lb.OperatingStatus, operStatusStyleFn},
		{"Provider", lb.Provider, nil},
		{"Description", lb.Description, nil},
	}

	for _, p := range props {
		if p.value == "" {
			continue
		}
		label := shared.StyleLabel.Render(p.label)
		var value string
		if p.style != nil {
			value = p.style(p.value).Render(shared.StatusIcon(p.value) + p.value)
		} else {
			value = shared.StyleValue.Render(p.value)
		}
		lines = append(lines, fmt.Sprintf("  %s %s", label, value))
	}

	// Listeners section
	if len(m.listeners) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("  %s",
			lipgloss.NewStyle().Bold(true).Foreground(shared.ColorSecondary).Render("--- Listeners ---")))
		lines = append(lines, "")

		// Build a pool name lookup
		poolNames := make(map[string]string)
		for _, p := range m.pools {
			poolNames[p.ID] = p.Name
		}

		for _, l := range m.listeners {
			poolName := poolNames[l.DefaultPoolID]
			if poolName == "" && l.DefaultPoolID != "" {
				poolName = l.DefaultPoolID[:min(8, len(l.DefaultPoolID))] + "..."
			}
			arrow := ""
			if poolName != "" {
				arrow = fmt.Sprintf(" -> %s", poolName)
			}
			lines = append(lines, fmt.Sprintf("    %s %s :%d%s",
				lipgloss.NewStyle().Foreground(shared.ColorHighlight).Render("->"),
				l.Protocol, l.ProtocolPort, arrow))
		}
	}

	// Pools section
	if len(m.pools) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("  %s",
			lipgloss.NewStyle().Bold(true).Foreground(shared.ColorSecondary).Render("--- Pools ---")))
		lines = append(lines, "")

		for _, p := range m.pools {
			lines = append(lines, fmt.Sprintf("    %s %s (%s, %s)",
				lipgloss.NewStyle().Foreground(shared.ColorHighlight).Render("->"),
				p.Name, p.LBMethod, p.Protocol))

			members := m.members[p.ID]
			if len(members) > 0 {
				lines = append(lines, fmt.Sprintf("      %s",
					shared.StyleLabel.Render("Members:")))
				for _, mem := range members {
					statusStyle := memberStatusStyle(mem.OperatingStatus)
					lines = append(lines, fmt.Sprintf("        %s:%d  w:%d  %s",
						mem.Address, mem.ProtocolPort, mem.Weight,
						statusStyle.Render(mem.OperatingStatus)))
				}
			}
		}
	}

	// Scroll
	viewHeight := m.height - 5
	if viewHeight < 1 {
		viewHeight = 1
	}
	if m.scroll > len(lines)-viewHeight {
		m.scroll = max(0, len(lines)-viewHeight)
	}

	end := m.scroll + viewHeight
	if end > len(lines) {
		end = len(lines)
	}

	for _, line := range lines[m.scroll:end] {
		b.WriteString(line + "\n")
	}

	return b.String()
}

func displayName(name, id string) string {
	if name != "" {
		return name
	}
	if len(id) > 12 {
		return id[:12] + "..."
	}
	return id
}

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
	}
	return lipgloss.NewStyle().Foreground(fg)
}

func (m Model) fetchDetail() tea.Cmd {
	client := m.client
	id := m.lbID
	return func() tea.Msg {
		ctx := context.Background()

		lb, err := loadbalancer.GetLoadBalancer(ctx, client, id)
		if err != nil {
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
		for _, p := range pls {
			mems, err := loadbalancer.ListMembers(ctx, client, p.ID)
			if err != nil {
				continue // best effort
			}
			members[p.ID] = mems
		}

		return lbDetailLoadedMsg{
			lb:        lb,
			listeners: lstnrs,
			pools:     pls,
			members:   members,
		}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return detailTickMsg{}
	})
}

// ForceRefresh triggers a manual reload of the load balancer detail.
func (m *Model) ForceRefresh() tea.Cmd {
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
	return "↑↓ scroll • ^d delete • R refresh • esc back • ? help"
}
