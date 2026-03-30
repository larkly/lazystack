package volumedetail

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/volume"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type volumeDetailLoadedMsg struct {
	vol        *volume.Volume
	serverName string
}

type volumeDetailErrMsg struct {
	err error
}

// Model is the volume detail view.
type Model struct {
	client          *gophercloud.ServiceClient
	computeClient   *gophercloud.ServiceClient
	volumeID        string
	volume          *volume.Volume
	serverName      string
	loading         bool
	spinner       spinner.Model
	width         int
	height        int
	scroll        int
	err           string
}

// New creates a volume detail model.
func New(client, computeClient *gophercloud.ServiceClient, volumeID string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:        client,
		computeClient: computeClient,
		volumeID:      volumeID,
		loading:       true,
		spinner:       s,
	}
}

// Init fetches the volume details.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchVolume())
}

// SelectedVolumeID returns the current volume ID.
func (m Model) SelectedVolumeID() string {
	return m.volumeID
}

// SelectedVolumeName returns the current volume name.
func (m Model) SelectedVolumeName() string {
	if m.volume != nil {
		if m.volume.Name != "" {
			return m.volume.Name
		}
		return m.volumeID
	}
	return m.volumeID
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case volumeDetailLoadedMsg:
		m.loading = false
		m.volume = msg.vol
		m.serverName = msg.serverName
		m.err = ""
		return m, nil

	case volumeDetailErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case shared.TickMsg:
		if m.loading {
			return m, nil
		}
		return m, m.fetchVolume()

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
				return shared.ViewChangeMsg{View: "volumelist"}
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

// View renders the volume detail.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Volume Detail")
	if m.loading {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if m.volume == nil {
		return b.String()
	}

	v := m.volume

	encrypted := "no"
	if v.Encrypted {
		encrypted = "yes"
	}
	multiattach := "no"
	if v.Multiattach {
		multiattach = "yes"
	}

	attachedServer := ""
	if v.AttachedServerID != "" {
		if m.serverName != "" {
			attachedServer = m.serverName
		} else {
			attachedServer = v.AttachedServerID
		}
	}

	updatedStr := ""
	if !v.Updated.IsZero() {
		updatedStr = v.Updated.Format("2006-01-02 15:04:05")
	}

	props := []struct {
		label string
		value string
	}{
		{"Name", v.Name},
		{"ID", v.ID},
		{"Status", v.Status},
		{"Size", fmt.Sprintf("%d GB", v.Size)},
		{"Type", v.VolumeType},
		{"Availability Zone", v.AZ},
		{"Bootable", v.Bootable},
		{"Encrypted", encrypted},
		{"Multiattach", multiattach},
		{"Description", v.Description},
		{"Created", v.Created.Format("2006-01-02 15:04:05")},
		{"Updated", updatedStr},
		{"Snapshot ID", v.SnapshotID},
		{"Source Volume ID", v.SourceVolID},
		{"Attached Server", attachedServer},
		{"Device", v.AttachedDevice},
	}

	lines := make([]string, 0, len(props)+len(v.Metadata))
	for _, p := range props {
		if p.value == "" {
			continue
		}
		label := shared.StyleLabel.Render(p.label)
		value := shared.StyleValue.Render(p.value)
		if p.label == "Status" {
			value = volumeStatusStyle(p.value).Render(shared.StatusIcon(p.value) + p.value)
		}
		lines = append(lines, fmt.Sprintf("  %s %s", label, value))
	}

	if len(v.Metadata) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("  %s", shared.StyleLabel.Render("Metadata")))
		keys := make([]string, 0, len(v.Metadata))
		for k := range v.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lines = append(lines, fmt.Sprintf("    %s = %s",
				lipgloss.NewStyle().Foreground(shared.ColorSecondary).Render(k),
				shared.StyleValue.Render(v.Metadata[k])))
		}
	}

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

func volumeStatusStyle(status string) lipgloss.Style {
	var fg = shared.ColorFg
	switch status {
	case "available":
		fg = shared.ColorSuccess
	case "in-use":
		fg = shared.ColorCyan
	case "creating", "downloading", "uploading", "extending":
		fg = shared.ColorWarning
	case "error", "error_deleting", "error_restoring":
		fg = shared.ColorError
	case "deleting":
		fg = shared.ColorMuted
	}
	return lipgloss.NewStyle().Foreground(fg)
}

func (m Model) fetchVolume() tea.Cmd {
	client := m.client
	computeClient := m.computeClient
	id := m.volumeID
	return func() tea.Msg {
		vol, err := volume.GetVolume(context.Background(), client, id)
		if err != nil {
			return volumeDetailErrMsg{err: err}
		}
		serverName := ""
		if vol.AttachedServerID != "" && computeClient != nil {
			srv, err := compute.GetServer(context.Background(), computeClient, vol.AttachedServerID)
			if err == nil && srv != nil {
				serverName = srv.Name
			}
		}
		return volumeDetailLoadedMsg{vol: vol, serverName: serverName}
	}
}

// ForceRefresh triggers a manual reload of the volume detail.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchVolume())
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	return "↑↓ scroll • ^d delete • ^a attach • ^t detach • R refresh • esc back • ? help"
}
