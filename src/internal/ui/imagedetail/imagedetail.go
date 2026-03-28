package imagedetail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type imageDetailLoadedMsg struct {
	image *image.Image
}

type imageDetailErrMsg struct {
	err error
}

type detailTickMsg struct{}

// Model is the image detail view.
type Model struct {
	client          *gophercloud.ServiceClient
	imageID         string
	image           *image.Image
	loading         bool
	spinner         spinner.Model
	width           int
	height          int
	scroll          int
	err             string
	refreshInterval time.Duration
}

// New creates an image detail model.
func New(client *gophercloud.ServiceClient, imageID string, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		client:          client,
		imageID:         imageID,
		loading:         true,
		spinner:         s,
		refreshInterval: refreshInterval,
	}
}

// Init fetches the image details.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchImage(), m.tickCmd())
}

// ImageID returns the current image ID.
func (m Model) ImageID() string {
	return m.imageID
}

// ImageName returns the current image name.
func (m Model) ImageName() string {
	if m.image != nil {
		return m.image.Name
	}
	return m.imageID
}

// ImageStatus returns the current image status.
func (m Model) ImageStatus() string {
	if m.image != nil {
		return m.image.Status
	}
	return ""
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case imageDetailLoadedMsg:
		m.loading = false
		m.image = msg.image
		m.err = ""
		return m, nil

	case imageDetailErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case detailTickMsg:
		return m, tea.Batch(m.fetchImage(), m.tickCmd())

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
				return shared.ViewChangeMsg{View: "imagelist"}
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

// View renders the image detail.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Image Detail")
	if m.loading {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if m.image == nil {
		return b.String()
	}

	img := m.image

	protectedStr := ""
	if img.Protected {
		protectedStr = "yes"
	}

	props := []struct {
		label string
		value string
	}{
		{"Name", img.Name},
		{"ID", img.ID},
		{"Status", img.Status},
		{"Visibility", img.Visibility},
		{"Size", formatSize(img.Size)},
		{"Disk Format", img.DiskFormat},
		{"Container Format", img.ContainerFormat},
		{"Min Disk", fmt.Sprintf("%d GB", img.MinDisk)},
		{"Min RAM", fmt.Sprintf("%d MB", img.MinRAM)},
		{"Checksum", img.Checksum},
		{"Owner", img.Owner},
		{"Protected", protectedStr},
		{"Tags", strings.Join(img.Tags, ", ")},
		{"Created", img.CreatedAt.Format("2006-01-02 15:04:05")},
		{"Updated", img.UpdatedAt.Format("2006-01-02 15:04:05")},
	}

	var lines []string
	for _, p := range props {
		if p.value == "" {
			continue
		}
		label := shared.StyleLabel.Render(p.label)
		value := shared.StyleValue.Render(p.value)
		if p.label == "Status" {
			value = statusStyle(p.value).Render(p.value)
		}
		lines = append(lines, fmt.Sprintf("  %s %s", label, value))
	}

	// Apply scroll
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

func statusStyle(status string) lipgloss.Style {
	var fg = shared.ColorFg
	switch status {
	case "active":
		fg = shared.ColorSuccess
	case "saving":
		fg = shared.ColorWarning
	case "queued", "importing":
		fg = shared.ColorCyan
	case "deactivated", "killed":
		fg = shared.ColorError
	case "deleted", "pending_delete":
		fg = shared.ColorMuted
	}
	return lipgloss.NewStyle().Foreground(fg)
}

func formatSize(bytes int64) string {
	if bytes == 0 {
		return "-"
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.0f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func (m Model) fetchImage() tea.Cmd {
	client := m.client
	id := m.imageID
	return func() tea.Msg {
		img, err := image.GetImage(context.Background(), client, id)
		if err != nil {
			return imageDetailErrMsg{err: err}
		}
		return imageDetailLoadedMsg{image: img}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return detailTickMsg{}
	})
}

// ForceRefresh triggers a manual reload of the image detail.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchImage())
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	return "↑↓ scroll • ^d delete • d deactivate • R refresh • esc back • ? help"
}
