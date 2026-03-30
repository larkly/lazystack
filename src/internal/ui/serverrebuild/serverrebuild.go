package serverrebuild

import (
	"context"
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type imagesLoadedMsg struct{ images []image.Image }
type fetchErrMsg struct{ err error }
type rebuildDoneMsg struct{ name string }
type rebuildErrMsg struct{ err error }

// Model is the rebuild image picker modal.
type Model struct {
	Active         bool
	computeClient  *gophercloud.ServiceClient
	imageClient    *gophercloud.ServiceClient
	serverID       string
	serverName     string
	currentImageID string
	images         []image.Image
	cursor         int
	filter         textinput.Model
	filtering      bool
	filtered       []image.Image
	loading        bool
	submitting     bool
	confirming     bool   // inline "are you sure?" step
	confirmImage   string // name of image pending confirmation
	spinner        spinner.Model
	width          int
	height         int
	err            string
}

// New creates a rebuild picker for a single server.
func New(computeClient, imageClient *gophercloud.ServiceClient, serverID, serverName, currentImageID string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	fi := textinput.New()
	fi.Prompt = "/ "
	fi.Placeholder = "filter..."
	fi.CharLimit = 64
	fi.SetVirtualCursor(false)

	return Model{
		Active:         true,
		computeClient:  computeClient,
		imageClient:    imageClient,
		serverID:       serverID,
		serverName:     serverName,
		currentImageID: currentImageID,
		loading:        true,
		spinner:        s,
		filter:         fi,
	}
}

// Init fetches images.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchImages())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case imagesLoadedMsg:
		m.loading = false
		m.images = msg.images
		m.filtered = msg.images
		return m, nil

	case fetchErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case rebuildDoneMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ServerActionMsg{Action: "Rebuilt", Name: msg.name}
		}

	case rebuildErrMsg:
		m.submitting = false
		m.confirming = false
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
		}
	case key.Matches(msg, shared.Keys.Down):
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case key.Matches(msg, shared.Keys.Filter):
		m.filtering = true
		m.filter.Focus()
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			sel := m.filtered[m.cursor]
			m.confirming = true
			m.confirmImage = sel.Name
			m.err = ""
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Confirm):
		if m.cursor >= 0 && m.cursor < len(m.filtered) {
			return m.doRebuild(m.filtered[m.cursor])
		}
		m.confirming = false
		return m, nil
	case key.Matches(msg, shared.Keys.Deny), key.Matches(msg, shared.Keys.Back):
		m.confirming = false
		m.confirmImage = ""
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
		m.filtered = m.images
	} else {
		m.filtered = nil
		for _, img := range m.images {
			if strings.Contains(strings.ToLower(img.Name), q) {
				m.filtered = append(m.filtered, img)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m Model) doRebuild(img image.Image) (Model, tea.Cmd) {
	m.submitting = true
	m.err = ""
	client := m.computeClient
	name := m.serverName
	id := m.serverID
	imageID := img.ID
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		shared.Debugf("[serverrebuild] rebuilding server %s (%s) with image %s", id, name, imageID)
		err := compute.RebuildServer(context.Background(), client, id, imageID)
		if err != nil {
			shared.Debugf("[serverrebuild] error rebuilding server %s: %v", id, err)
			return rebuildErrMsg{err: err}
		}
		shared.Debugf("[serverrebuild] rebuilt server %s (%s)", id, name)
		return rebuildDoneMsg{name: name}
	})
}

func (m Model) listHeight() int {
	h := m.height - 14
	if h < 3 {
		h = 3
	}
	if h > 15 {
		h = 15
	}
	return h
}

// View renders the rebuild modal overlay.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleModalTitle.Render(fmt.Sprintf("Rebuild %s", m.serverName))
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
		b.WriteString(shared.StyleHelp.Render("No images found.") + "\n")
	} else if !m.loading {
		maxName := 4
		for _, img := range m.filtered {
			n := len(img.Name)
			if img.ID == m.currentImageID {
				n += 2 // " ★"
			}
			if n > maxName {
				maxName = n
			}
		}
		if maxName > 50 {
			maxName = 50
		}

		header := fmt.Sprintf("  %-*s %6s %6s", maxName, "Name", "MinDsk", "MinRAM")
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
			img := m.filtered[i]
			isCurrent := img.ID == m.currentImageID
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
			name := img.Name
			if len(name) > maxName-2 && isCurrent {
				name = name[:maxName-4] + "…"
			} else if len(name) > maxName {
				name = name[:maxName-1] + "…"
			}
			if isCurrent {
				name += " ★"
			}
			diskStr := fmt.Sprintf("%dGB", img.MinDisk)
			ramStr := fmt.Sprintf("%dMB", img.MinRAM)
			if img.MinDisk == 0 {
				diskStr = "-"
			}
			if img.MinRAM == 0 {
				ramStr = "-"
			}
			line := fmt.Sprintf("%-*s %6s %6s", maxName, name, diskStr, ramStr)
			b.WriteString(cursor + style.Render(line) + "\n")
		}

		if len(m.filtered) > vh {
			b.WriteString(shared.StyleHelp.Render(fmt.Sprintf("  %d/%d images", m.cursor+1, len(m.filtered))) + "\n")
		}
	}

	b.WriteString("\n")
	if m.confirming {
		warn := lipgloss.NewStyle().Foreground(shared.ColorError).Bold(true).Render(
			fmt.Sprintf("Rebuild with %s? This is destructive! [y/n]", m.confirmImage))
		b.WriteString(warn)
	} else {
		b.WriteString(shared.StyleHelp.Render("↑↓ navigate • enter select • / filter • esc cancel"))
	}

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

func (m Model) fetchImages() tea.Cmd {
	client := m.imageClient
	return func() tea.Msg {
		imgs, err := image.ListImages(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return imagesLoadedMsg{images: imgs}
	}
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}
