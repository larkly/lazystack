package serverresize

import (
	"context"
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type flavorsLoadedMsg struct{ flavors []compute.Flavor }
type fetchErrMsg struct{ err error }
type resizeDoneMsg struct{ name string }
type resizeErrMsg struct{ err error }

// Model is the resize flavor picker modal.
type Model struct {
	Active        bool
	client        *gophercloud.ServiceClient
	serverID      string
	serverIDs     []string // for bulk resize
	serverName    string
	currentFlavor string
	flavors    []compute.Flavor
	cursor     int
	filter     textinput.Model
	filtering  bool
	filtered   []compute.Flavor
	loading    bool
	submitting bool
	spinner    spinner.Model
	width      int
	height     int
	err        string
}

// NewBulk creates a resize picker for multiple servers.
func NewBulk(client *gophercloud.ServiceClient, serverIDs []string, currentFlavor string) Model {
	m := New(client, "", fmt.Sprintf("%d servers", len(serverIDs)), currentFlavor)
	m.serverIDs = serverIDs
	return m
}

// New creates a resize picker. currentFlavor is the flavor name to exclude.
func New(client *gophercloud.ServiceClient, serverID, serverName, currentFlavor string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	fi := textinput.New()
	fi.Prompt = "/ "
	fi.Placeholder = "filter..."
	fi.CharLimit = 64
	fi.SetVirtualCursor(false)

	return Model{
		Active:        true,
		client:        client,
		serverID:      serverID,
		serverName:    serverName,
		currentFlavor: currentFlavor,
		loading:    true,
		spinner:    s,
		filter:     fi,
	}
}

// Init fetches flavors.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchFlavors())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case flavorsLoadedMsg:
		m.loading = false
		m.flavors = msg.flavors
		m.filtered = msg.flavors
		return m, nil

	case fetchErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case resizeDoneMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ServerActionMsg{Action: "Resize", Name: msg.name}
		}

	case resizeErrMsg:
		m.submitting = false
		m.err = msg.err.Error()
		return m, nil

	case spinner.TickMsg:
		if m.loading || m.submitting {
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
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.updateNormal(msg)
	}
	return m, nil
}

func (m Model) updateNormal(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.Active = false
		return m, nil
	case key.Matches(msg, shared.Keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.ensureVisible()
		}
	case key.Matches(msg, shared.Keys.Down):
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.ensureVisible()
		}
	case key.Matches(msg, shared.Keys.Filter):
		m.filtering = true
		m.filter.Focus()
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			return m.doResize(m.filtered[m.cursor])
		}
	}
	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering = false
		m.filter.SetValue("")
		m.filter.Blur()
		m.applyFilter()
		return m, nil
	case "enter":
		m.filtering = false
		m.filter.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.applyFilter()
	return m, cmd
}

func (m *Model) applyFilter() {
	q := strings.ToLower(m.filter.Value())
	if q == "" {
		m.filtered = m.flavors
	} else {
		m.filtered = nil
		for _, f := range m.flavors {
			if strings.Contains(strings.ToLower(f.Name), q) {
				m.filtered = append(m.filtered, f)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m *Model) ensureVisible() {
	vh := m.listHeight()
	scrollStart := 0
	if m.cursor >= vh {
		scrollStart = m.cursor - vh + 1
	}
	_ = scrollStart // scroll is implicit via start calculation in View
}

func (m Model) doResize(flavor compute.Flavor) (Model, tea.Cmd) {
	m.submitting = true
	m.err = ""
	client := m.client
	name := m.serverName
	flavorID := flavor.ID

	// Bulk resize
	if len(m.serverIDs) > 0 {
		ids := m.serverIDs
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			shared.Debugf("[serverresize] resizing %d servers to flavor %s", len(ids), flavorID)
			var errs []string
			for _, id := range ids {
				err := compute.ResizeServer(context.Background(), client, id, flavorID)
				if err != nil {
					errs = append(errs, err.Error())
				}
			}
			if len(errs) > 0 {
				shared.Debugf("[serverresize] error resizing servers: %s", strings.Join(errs, "; "))
				return resizeErrMsg{err: fmt.Errorf("%s", strings.Join(errs, "; "))}
			}
			shared.Debugf("[serverresize] resized %d servers to flavor %s", len(ids), flavorID)
			return resizeDoneMsg{name: name}
		})
	}

	// Single resize
	id := m.serverID
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		shared.Debugf("[serverresize] resizing server %s (%s) to flavor %s", id, name, flavorID)
		err := compute.ResizeServer(context.Background(), client, id, flavorID)
		if err != nil {
			shared.Debugf("[serverresize] error resizing server %s: %v", id, err)
			return resizeErrMsg{err: err}
		}
		shared.Debugf("[serverresize] resized server %s (%s)", id, name)
		return resizeDoneMsg{name: name}
	})
}

func (m Model) listHeight() int {
	// Modal inner height minus title, header, filter, hint, padding
	h := m.height - 14
	if h < 3 {
		h = 3
	}
	if h > 15 {
		h = 15
	}
	return h
}

// View renders the resize modal overlay.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleModalTitle.Render(fmt.Sprintf("Resize %s", m.serverName))
	if m.loading || m.submitting {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("⚠ "+m.err) + "\n\n")
	}

	if m.filtering {
		b.WriteString(m.filter.View() + "\n")
	} else if m.filter.Value() != "" {
		b.WriteString(shared.StyleHelp.Render(fmt.Sprintf("filter: %s", m.filter.Value())) + "\n")
	}

	if len(m.filtered) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("No flavors found.") + "\n")
	} else if !m.loading {
		// Find longest flavor name to size columns
		maxName := 4 // minimum "Name"
		for _, f := range m.filtered {
			n := len(f.Name)
			if f.Name == m.currentFlavor {
				n += 2 // " ★"
			}
			if n > maxName {
				maxName = n
			}
		}

		// Header
		header := fmt.Sprintf("  %-*s %5s %7s %5s", maxName, "Name", "vCPU", "RAM", "Disk")
		b.WriteString(shared.StyleHeader.Render(header) + "\n")

		vh := m.listHeight()
		start := 0
		if m.cursor >= vh {
			start = m.cursor - vh + 1
		}
		end := start + vh
		if end > len(m.filtered) {
			end = len(m.filtered)
		}

		for i := start; i < end; i++ {
			f := m.filtered[i]
			isCurrent := f.Name == m.currentFlavor
			cursor := "  "
			style := lipgloss.NewStyle().Foreground(shared.ColorFg)
			if isCurrent {
				style = lipgloss.NewStyle().Foreground(shared.ColorMuted)
			}
			if i == m.cursor {
				cursor = "▸ "
				if isCurrent {
					style = style.Foreground(shared.ColorMuted).Bold(true)
				} else {
					style = style.Foreground(shared.ColorHighlight).Bold(true)
				}
			}
			name := f.Name
			if isCurrent {
				name += " ★"
			}
			line := fmt.Sprintf("%-*s %5d %5dMB %4dGB", maxName, name, f.VCPUs, f.RAM, f.Disk)
			b.WriteString(cursor + style.Render(line) + "\n")
		}

		if len(m.filtered) > vh {
			b.WriteString(shared.StyleHelp.Render(fmt.Sprintf("  %d/%d flavors", m.cursor+1, len(m.filtered))) + "\n")
		}
	}

	b.WriteString("\n")
	hint := shared.StyleHelp.Render("↑↓ navigate • enter resize • / filter • esc cancel")
	b.WriteString(hint)

	// Size modal to fit content + border/padding (8 chars)
	contentWidth := lipgloss.Width(b.String())
	modalWidth := contentWidth + 8
	maxWidth := m.width - 4
	if modalWidth > maxWidth {
		modalWidth = maxWidth
	}
	if modalWidth < 40 {
		modalWidth = 40
	}
	box := shared.StyleModal.Width(modalWidth).Render(b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) fetchFlavors() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		flavors, err := compute.ListFlavors(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return flavorsLoadedMsg{flavors: flavors}
	}
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}
