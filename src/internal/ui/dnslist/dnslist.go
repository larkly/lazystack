package dnslist

import (
	"context"
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
)

type zonesLoadedMsg struct {
	zones []zones.Zone
}

type zonesErrMsg struct {
	err error
}

type recordsetsLoadedMsg struct {
	zoneID    string
	recordsets []recordsets.RecordSet
}

type recordsetsErrMsg struct {
	err error
}

// Model is the DNS zone/record list viewer.
type Model struct {
	client     *gophercloud.ServiceClient
	zones      []zones.Zone
	recordsets []recordsets.RecordSet
	selectedZone *zones.Zone
	cursor     int
	scroll     int
	width      int
	height     int
	loading    bool
	spinner    spinner.Model
	err        string
}

// New creates a DNS list model.
func New(client *gophercloud.ServiceClient) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:  client,
		loading: true,
		spinner: s,
	}
}

// Init fetches zones.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchZones())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case zonesLoadedMsg:
		m.loading = false
		m.zones = msg.zones
		m.err = ""
		m.cursor = 0
		m.scroll = 0
		if len(m.zones) > 0 {
			m.selectedZone = &m.zones[0]
			return m, m.fetchRecordsets(m.zones[0].ID)
		}
		return m, nil

	case zonesErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case recordsetsLoadedMsg:
		m.loading = false
		m.recordsets = msg.recordsets
		m.err = ""
		return m, nil

	case recordsetsErrMsg:
		m.loading = false
		m.err = msg.err.Error()
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
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Back):
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "serverlist"}
			}
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.scroll {
					m.scroll = m.cursor
				}
			}
			// Update selected zone
			if m.cursor < len(m.zones) {
				m.selectedZone = &m.zones[m.cursor]
				m.loading = true
				return m, tea.Batch(m.spinner.Tick, m.fetchRecordsets(m.zones[m.cursor].ID))
			}
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.zones)-1 {
				m.cursor++
				visible := m.visibleRows()
				if m.cursor >= m.scroll+visible {
					m.scroll = m.cursor - visible + 1
				}
				// Update selected zone
				m.selectedZone = &m.zones[m.cursor]
				m.loading = true
				return m, tea.Batch(m.spinner.Tick, m.fetchRecordsets(m.zones[m.cursor].ID))
			}
		case key.Matches(msg, shared.Keys.PageUp):
			m.cursor -= m.visibleRows()
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.scroll = m.cursor
			if m.cursor < len(m.zones) {
				m.selectedZone = &m.zones[m.cursor]
				m.loading = true
				return m, tea.Batch(m.spinner.Tick, m.fetchRecordsets(m.zones[m.cursor].ID))
			}
		case key.Matches(msg, shared.Keys.PageDown):
			m.cursor += m.visibleRows()
			if m.cursor >= len(m.zones) {
				m.cursor = len(m.zones) - 1
			}
			visible := m.visibleRows()
			if m.cursor >= m.scroll+visible {
				m.scroll = m.cursor - visible + 1
			}
			if m.cursor < len(m.zones) {
				m.selectedZone = &m.zones[m.cursor]
				m.loading = true
				return m, tea.Batch(m.spinner.Tick, m.fetchRecordsets(m.zones[m.cursor].ID))
			}
		}
	}
	return m, nil
}

func (m Model) visibleRows() int {
	h := m.height - 6 // title + status bar + header + zone header + padding
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

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.zones) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No DNS zones found.") + "\n")
		return b.String()
	}

	// Zone list header
	header := lipgloss.NewStyle().Bold(true).Foreground(shared.ColorMuted)
	headerRow := fmt.Sprintf("  %-40s %-10s %-12s",
		"Zone Name", "Type", "Records")
	b.WriteString(header.Render(headerRow) + "\n")

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

		zoneType := "PRIMARY"
		if z.Type != "" {
			zoneType = z.Type
		}

		row := fmt.Sprintf("%s%-40s %-10s %s%s",
			prefix,
			truncate(z.Name, 39),
			zoneType,
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
					truncate(name, 34),
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
	return "↑↓ navigate zones • esc back • R refresh • ? help"
}

// ForceRefresh triggers a reload.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchZones())
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m Model) fetchZones() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		allPages, err := zones.List(client, nil).AllPages(context.Background())
		if err != nil {
			return zonesErrMsg{err: err}
		}
		z, err := zones.ExtractZones(allPages)
		if err != nil {
			return zonesErrMsg{err: err}
		}
		return zonesLoadedMsg{zones: z}
	}
}

func (m Model) fetchRecordsets(zoneID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		allPages, err := recordsets.ListByZone(client, zoneID, nil).AllPages(context.Background())
		if err != nil {
			return recordsetsErrMsg{err: err}
		}
		rs, err := recordsets.ExtractRecordSets(allPages)
		if err != nil {
			return recordsetsErrMsg{err: err}
		}
		return recordsetsLoadedMsg{zoneID: zoneID, recordsets: rs}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
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
