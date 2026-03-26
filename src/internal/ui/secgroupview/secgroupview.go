package secgroupview

import (
	"context"
	"fmt"
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

type sgLoadedMsg struct{ groups []network.SecurityGroup }
type sgErrMsg struct{ err error }
type tickMsg struct{}

// Model is the security group viewer.
type Model struct {
	client          *gophercloud.ServiceClient
	groups          []network.SecurityGroup
	groupNames      map[string]string // ID → name for resolving remote group references
	cursor          int
	expanded        map[int]bool // which groups are expanded to show rules
	ruleCursor      int          // cursor within expanded group's rules (-1 = on group header)
	inRules         bool         // true when navigating rules within an expanded group
	width           int
	height          int
	loading         bool
	spinner         spinner.Model
	err             string
	scrollOff       int
	refreshInterval time.Duration
}

// New creates a security group view model.
func New(client *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:          client,
		loading:         true,
		spinner:         s,
		expanded:        make(map[int]bool),
		refreshInterval: refreshInterval,
	}
}

// Init starts the initial fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchGroups(), m.tickCmd())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sgLoadedMsg:
		m.loading = false
		m.groups = msg.groups
		m.groupNames = make(map[string]string)
		for _, g := range msg.groups {
			m.groupNames[g.ID] = g.Name
		}
		m.err = ""
		return m, nil

	case sgErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.fetchGroups(), m.tickCmd())

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
		case key.Matches(msg, shared.Keys.Up):
			if m.inRules {
				if m.ruleCursor > 0 {
					m.ruleCursor--
				} else {
					m.inRules = false
				}
			} else if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, shared.Keys.Down):
			if m.inRules {
				sg := m.groups[m.cursor]
				if m.ruleCursor < len(sg.Rules)-1 {
					m.ruleCursor++
				} else {
					// Move to next group
					m.inRules = false
					if m.cursor < len(m.groups)-1 {
						m.cursor++
					}
				}
			} else if m.expanded[m.cursor] && len(m.groups[m.cursor].Rules) > 0 {
				// Enter rules navigation
				m.inRules = true
				m.ruleCursor = 0
			} else if m.cursor < len(m.groups)-1 {
				m.cursor++
			}
		case key.Matches(msg, shared.Keys.PageDown):
			if !m.inRules {
				m.cursor += m.height - 5
				if m.cursor >= len(m.groups) {
					m.cursor = len(m.groups) - 1
				}
				if m.cursor < 0 {
					m.cursor = 0
				}
			}
		case key.Matches(msg, shared.Keys.PageUp):
			if !m.inRules {
				m.cursor -= m.height - 5
				if m.cursor < 0 {
					m.cursor = 0
				}
			}
		case key.Matches(msg, shared.Keys.Enter):
			if !m.inRules {
				wasExpanded := m.expanded[m.cursor]
				m.expanded[m.cursor] = !wasExpanded
				if wasExpanded {
					m.inRules = false
				}
			}
		case key.Matches(msg, shared.Keys.Back):
			if m.inRules {
				m.inRules = false
				return m, nil
			}
		}
	}
	return m, nil
}

// SelectedRule returns the currently selected rule ID, or "" if none.
func (m Model) SelectedRule() string {
	if !m.inRules {
		return ""
	}
	if m.cursor < 0 || m.cursor >= len(m.groups) {
		return ""
	}
	sg := m.groups[m.cursor]
	if m.ruleCursor < 0 || m.ruleCursor >= len(sg.Rules) {
		return ""
	}
	return sg.Rules[m.ruleCursor].ID
}

// SelectedGroupName returns the name of the group under the cursor.
func (m Model) SelectedGroupName() string {
	if m.cursor >= 0 && m.cursor < len(m.groups) {
		return m.groups[m.cursor].Name
	}
	return ""
}

// SelectedGroupID returns the ID of the currently selected group.
func (m Model) SelectedGroupID() string {
	if m.cursor >= 0 && m.cursor < len(m.groups) {
		return m.groups[m.cursor].ID
	}
	return ""
}

// View renders the security group viewer.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Security Groups")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.groups))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.groups) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No security groups found.") + "\n")
		return b.String()
	}

	for i, sg := range m.groups {
		cursor := "  "
		if i == m.cursor {
			cursor = "▸ "
		}

		nameStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.cursor {
			nameStyle = nameStyle.Foreground(shared.ColorHighlight).Bold(true)
		}

		expandIcon := "▶"
		if m.expanded[i] {
			expandIcon = "▼"
		}

		rulesCount := fmt.Sprintf(" (%d rules)", len(sg.Rules))
		desc := ""
		if sg.Description != "" {
			desc = shared.StyleHelp.Render(" — " + sg.Description)
		}

		b.WriteString(cursor + expandIcon + " " + nameStyle.Render(sg.Name) +
			shared.StyleHelp.Render(rulesCount) + desc + "\n")

		if m.expanded[i] {
			if len(sg.Rules) == 0 {
				b.WriteString(shared.StyleHelp.Render("      No rules") + "\n")
			}
			for ri, r := range sg.Rules {
				ruleSel := m.inRules && i == m.cursor && ri == m.ruleCursor
				b.WriteString(m.renderRule(r, ruleSel) + "\n")
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m Model) renderRule(r network.SecurityRule, selected bool) string {
	dir := lipgloss.NewStyle().Width(8).Foreground(shared.ColorSecondary).Render(r.Direction)

	proto := r.Protocol
	if proto == "" {
		proto = "any"
	}

	ports := ""
	if r.PortRangeMin == 0 && r.PortRangeMax == 0 {
		ports = "all"
	} else if r.PortRangeMin == r.PortRangeMax {
		ports = fmt.Sprintf("%d", r.PortRangeMin)
	} else {
		ports = fmt.Sprintf("%d-%d", r.PortRangeMin, r.PortRangeMax)
	}

	remote := r.RemoteIPPrefix
	if remote == "" && r.RemoteGroupID != "" {
		if name, ok := m.groupNames[r.RemoteGroupID]; ok {
			remote = "group:" + name
		} else {
			remote = "group:" + r.RemoteGroupID[:8] + "…"
		}
	}
	if remote == "" {
		remote = "any"
	}

	ether := lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(r.EtherType)

	prefix := "      "
	line := fmt.Sprintf("%s%s %-6s %-10s %-20s %s", prefix, dir, proto, ports, remote, ether)
	if selected {
		line = fmt.Sprintf("    ▸ %s %-6s %-10s %-20s %s", dir, proto, ports, remote, ether)
		line = lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true).Render(line)
	}
	return line
}

func (m Model) fetchGroups() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		groups, err := network.ListSecurityGroups(context.Background(), client)
		if err != nil {
			return sgErrMsg{err: err}
		}
		return sgLoadedMsg{groups: groups}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// ForceRefresh triggers a manual reload of the security groups.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchGroups())
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	if m.inRules {
		return "↑↓ navigate rules • ^n add rule • ^d delete rule • esc back to groups • R refresh • ? help"
	}
	return "↑↓ navigate • enter expand/collapse • ^n add rule • R refresh • 1-5/←→ switch tab • ? help"
}
