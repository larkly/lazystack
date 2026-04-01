package quotaview

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/quota"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

const (
	barWidth      = 18
	cacheDuration = 30 * time.Second
)

type quotaLoadedMsg struct {
	compute []quota.QuotaUsage
	network []quota.QuotaUsage
	volume  []quota.QuotaUsage
	err     error
}

// Model is the quota overlay.
type Model struct {
	Visible       bool
	Width         int
	Height        int
	computeClient *gophercloud.ServiceClient
	networkClient *gophercloud.ServiceClient
	blockClient   *gophercloud.ServiceClient
	projectID     string
	compute       []quota.QuotaUsage
	network       []quota.QuotaUsage
	volume        []quota.QuotaUsage
	loading       bool
	spinner       spinner.Model
	err           string
	scroll        int
	lastFetch     time.Time
	lines         []string
}

// New creates a new quota overlay model.
func New() Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{
		spinner: sp,
	}
}

// SetClients updates the service clients and project ID.
func (m *Model) SetClients(compute, network, block *gophercloud.ServiceClient, projectID string) {
	m.computeClient = compute
	m.networkClient = network
	m.blockClient = block
	m.projectID = projectID
	// Invalidate cache when clients change
	m.lastFetch = time.Time{}
}

// SetProjectID updates the project ID for quota lookups.
func (m *Model) SetProjectID(id string) {
	if id != m.projectID {
		m.projectID = id
		m.lastFetch = time.Time{}
	}
}

// Open triggers a fetch if cache is stale. Returns a command.
func (m *Model) Open() tea.Cmd {
	m.Visible = true
	m.scroll = 0
	if time.Since(m.lastFetch) > cacheDuration {
		m.loading = true
		m.err = ""
		return tea.Batch(m.spinner.Tick, m.fetchQuotas())
	}
	return nil
}

func (m *Model) fetchQuotas() tea.Cmd {
	computeClient := m.computeClient
	networkClient := m.networkClient
	blockClient := m.blockClient
	projectID := m.projectID

	return func() tea.Msg {
		if projectID == "" {
			return quotaLoadedMsg{err: fmt.Errorf("no project selected")}
		}
		ctx := context.Background()
		var (
			computeQuotas []quota.QuotaUsage
			networkQuotas []quota.QuotaUsage
			volumeQuotas  []quota.QuotaUsage
			firstErr      error
		)

		if computeClient != nil {
			cq, err := quota.GetComputeQuotas(ctx, computeClient, projectID)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
			} else {
				computeQuotas = cq
			}
		}

		if networkClient != nil {
			nq, err := quota.GetNetworkQuotas(ctx, networkClient, projectID)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
			} else {
				networkQuotas = nq
			}
		}

		if blockClient != nil {
			vq, err := quota.GetVolumeQuotas(ctx, blockClient, projectID)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
			} else {
				volumeQuotas = vq
			}
		}

		// Only return error if we got nothing at all
		if computeQuotas == nil && networkQuotas == nil && volumeQuotas == nil && firstErr != nil {
			return quotaLoadedMsg{err: firstErr}
		}

		return quotaLoadedMsg{
			compute: computeQuotas,
			network: networkQuotas,
			volume:  volumeQuotas,
		}
	}
}

// Update handles messages for the quota overlay.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case msg.String() == "Q", key.Matches(msg, shared.Keys.Back):
			m.Visible = false
			m.scroll = 0
			return m, nil
		case key.Matches(msg, shared.Keys.Down):
			maxScroll := len(m.lines) - m.viewHeight()
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scroll < maxScroll {
				m.scroll++
			}
		case key.Matches(msg, shared.Keys.Up):
			if m.scroll > 0 {
				m.scroll--
			}
		case key.Matches(msg, shared.Keys.PageDown):
			maxScroll := len(m.lines) - m.viewHeight()
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.scroll += m.viewHeight()
			if m.scroll > maxScroll {
				m.scroll = maxScroll
			}
		case key.Matches(msg, shared.Keys.PageUp):
			m.scroll -= m.viewHeight()
			if m.scroll < 0 {
				m.scroll = 0
			}
		}

	case quotaLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.err = ""
			m.compute = msg.compute
			m.network = msg.network
			m.volume = msg.volume
			m.lastFetch = time.Now()
		}
		m.buildLines()
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
	}
	return m, nil
}

func (m *Model) buildLines() {
	m.lines = nil

	if m.err != "" {
		m.lines = append(m.lines,
			lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: "+m.err))
		return
	}

	if len(m.compute) > 0 {
		m.lines = append(m.lines, sectionHeader("Compute"))
		for _, q := range m.compute {
			m.lines = append(m.lines, formatQuotaLine(q))
		}
		m.lines = append(m.lines, "")
	}

	if len(m.network) > 0 {
		m.lines = append(m.lines, sectionHeader("Network"))
		for _, q := range m.network {
			m.lines = append(m.lines, formatQuotaLine(q))
		}
		m.lines = append(m.lines, "")
	}

	if len(m.volume) > 0 {
		m.lines = append(m.lines, sectionHeader("Block Storage"))
		for _, q := range m.volume {
			m.lines = append(m.lines, formatQuotaLine(q))
		}
		m.lines = append(m.lines, "")
	}

	if len(m.lines) == 0 {
		m.lines = append(m.lines,
			lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("No quota data available"))
	}
}

func sectionHeader(name string) string {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(shared.ColorSecondary).
		Render(fmt.Sprintf("--- %s ---", name))
}

func formatQuotaLine(q quota.QuotaUsage) string {
	name := fmt.Sprintf("%-17s", q.Resource)

	if q.Limit < 0 {
		// Unlimited
		return fmt.Sprintf("  %s %d / unlimited",
			name, q.Used)
	}

	bar := renderBar(q.Used, q.Limit)
	return fmt.Sprintf("  %s %s  %d/%d",
		name, bar, q.Used, q.Limit)
}

func renderBar(used, limit int) string {
	if limit == 0 {
		return strings.Repeat(".", barWidth)
	}

	pct := float64(used) / float64(limit)
	filled := int(pct * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	var barColor color.Color
	switch {
	case pct > 0.9:
		barColor = shared.ColorError // red
	case pct > 0.7:
		barColor = shared.ColorWarning // yellow
	default:
		barColor = shared.ColorSuccess // green
	}

	filledStyle := lipgloss.NewStyle().Foreground(barColor)
	emptyStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)

	return filledStyle.Render(strings.Repeat("\u2588", filled)) +
		emptyStyle.Render(strings.Repeat("\u2591", barWidth-filled))
}

func (m Model) viewHeight() int {
	h := m.Height - 8
	if h < 3 {
		h = 3
	}
	return h
}

// Render returns the quota overlay content.
func (m Model) Render() string {
	title := shared.StyleModalTitle.Render("Resource Quotas")

	if m.loading {
		content := title + "\n\n" + m.spinner.View() + " Loading quotas..."
		hint := shared.StyleHelp.Render(" Q or esc to close")
		content += "\n\n" + hint
		box := shared.StyleModal.Width(56).Render(content)
		return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
	}

	vh := m.viewHeight()
	end := m.scroll + vh
	if end > len(m.lines) {
		end = len(m.lines)
	}
	start := m.scroll
	if start > len(m.lines) {
		start = len(m.lines)
	}

	visible := strings.Join(m.lines[start:end], "\n")

	scrollHint := ""
	if m.scroll > 0 || end < len(m.lines) {
		scrollHint = shared.StyleHelp.Render(" \u2191\u2193 scroll \u2022")
	}

	hint := scrollHint + shared.StyleHelp.Render(" Q or esc to close")

	content := title + "\n\n" + visible + "\n\n" + hint
	box := shared.StyleModal.Width(56).Render(content)

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
}
