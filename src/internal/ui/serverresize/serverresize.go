package serverresize

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
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
	flavors       []compute.Flavor
	cursor        int
	filter        textinput.Model
	filtering     bool
	filtered      []compute.Flavor
	loading       bool
	submitting    bool
	confirming    bool           // bulk resize awaiting explicit confirmation
	confirmFlavor compute.Flavor // target flavor while confirming
	spinner       spinner.Model
	width         int
	height        int
	err           string

	// Track, when set, is called as a resize is submitted with the target
	// server IDs and the request command. It returns the command to run in
	// its place (for example one holding per-server in-flight locks for its
	// lifetime) or, when a target is busy, a reason to refuse the request.
	Track func(ids []string, cmd tea.Cmd) (tea.Cmd, string)
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
		loading:       true,
		spinner:       s,
		filter:        fi,
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
		// While a resize is in flight, ignore all input so the request
		// cannot be repeated and its outcome cannot be discarded.
		if m.submitting {
			return m, nil
		}
		if m.confirming {
			return m.updateConfirm(msg)
		}
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
			if len(m.serverIDs) > 0 {
				// Bulk resize affects many servers; confirm first.
				m.confirming = true
				m.confirmFlavor = m.filtered[m.cursor]
				m.err = ""
				return m, nil
			}
			return m.doResize(m.filtered[m.cursor])
		}
	}
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Confirm), key.Matches(msg, shared.Keys.Enter):
		m.confirming = false
		return m.doResize(m.confirmFlavor)
	case key.Matches(msg, shared.Keys.Deny), key.Matches(msg, shared.Keys.Back):
		m.confirming = false
		return m, nil
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
	client := m.client
	name := m.serverName
	flavorID := flavor.ID

	var ids []string
	var cmd tea.Cmd
	if len(m.serverIDs) > 0 {
		// Bulk resize
		ids = m.serverIDs
		cmd = func() tea.Msg {
			shared.Debugf("[serverresize] resizing %d servers to flavor %s", len(ids), flavorID)
			var errs []string
			for _, id := range ids {
				ctx, cancel := shared.RequestCtx()
				err := compute.ResizeServer(ctx, client, id, flavorID)
				cancel()
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
		}
	} else {
		// Single resize
		id := m.serverID
		ids = []string{id}
		cmd = func() tea.Msg {
			ctx, cancel := shared.RequestCtx()
			defer cancel()
			shared.Debugf("[serverresize] resizing server %s (%s) to flavor %s", id, name, flavorID)
			err := compute.ResizeServer(ctx, client, id, flavorID)
			if err != nil {
				shared.Debugf("[serverresize] error resizing server %s: %v", id, err)
				return resizeErrMsg{err: err}
			}
			shared.Debugf("[serverresize] resized server %s (%s)", id, name)
			return resizeDoneMsg{name: name}
		}
	}

	if m.Track != nil {
		tracked, busy := m.Track(ids, cmd)
		if busy != "" {
			m.err = busy
			return m, nil
		}
		cmd = tracked
	}
	m.submitting = true
	m.err = ""
	return m, tea.Batch(m.spinner.Tick, cmd)
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

	if m.confirming {
		f := m.confirmFlavor
		b.WriteString(fmt.Sprintf("Resize %d servers to flavor %s (%d vCPU, %dMB RAM, %dGB disk)?\n",
			len(m.serverIDs), f.Name, f.VCPUs, f.RAM, f.Disk))
		b.WriteString("\n")
		b.WriteString(shared.StyleHelp.Render("y/enter confirm • n/esc cancel"))
		return m.renderBox(b.String())
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

	return m.renderBox(b.String())
}

func (m Model) renderBox(content string) string {
	// Size modal to fit content + border/padding (8 chars)
	contentWidth := lipgloss.Width(content)
	modalWidth := contentWidth + 8
	maxWidth := m.width - 4
	if modalWidth > maxWidth {
		modalWidth = maxWidth
	}
	if modalWidth < 40 {
		modalWidth = 40
	}
	box := shared.StyleModal.Width(modalWidth).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) fetchFlavors() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		flavors, err := compute.ListFlavors(ctx, client)
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
