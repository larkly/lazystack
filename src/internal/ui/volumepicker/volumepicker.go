package volumepicker

import (
	"context"
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/volume"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type volumesLoadedMsg struct{ volumes []volume.Volume }
type fetchErrMsg struct{ err error }
type attachDoneMsg struct{ serverName, volumeName string }
type attachErrMsg struct{ err error }

// Model is the volume picker modal for attaching a volume to a server.
type Model struct {
	Active        bool
	computeClient *gophercloud.ServiceClient
	blockClient   *gophercloud.ServiceClient
	serverID      string
	serverName    string
	volumes       []volume.Volume
	filtered      []volume.Volume
	cursor        int
	loading       bool
	submitting    bool
	spinner       spinner.Model
	filter        string
	width         int
	height        int
	err           string
	scrollOff     int
}

// New creates a volume picker for attaching a volume to the given server.
func New(computeClient, blockClient *gophercloud.ServiceClient, serverID, serverName string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		Active:        true,
		computeClient: computeClient,
		blockClient:   blockClient,
		serverID:      serverID,
		serverName:    serverName,
		loading:       true,
		spinner:       s,
	}
}

// Init fetches available volumes.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[volumepicker] Init() serverID=%s serverName=%q", m.serverID, m.serverName)
	return tea.Batch(m.spinner.Tick, m.fetchVolumes())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case volumesLoadedMsg:
		m.loading = false
		m.volumes = msg.volumes
		m.applyFilter()
		shared.Debugf("[volumepicker] loaded %d volumes", len(msg.volumes))
		return m, nil

	case fetchErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		shared.Debugf("[volumepicker] error: %v", msg.err)
		return m, nil

	case attachDoneMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{
				Action: "Attached volume",
				Name:   fmt.Sprintf("%s → %s", msg.volumeName, msg.serverName),
			}
		}

	case attachErrMsg:
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
		if m.loading || m.submitting {
			return m, nil
		}
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
		case key.Matches(msg, shared.Keys.Enter):
			if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
				vol := m.filtered[m.cursor]
				shared.Debugf("[volumepicker] selected volume=%q id=%s", vol.Name, vol.ID)
				m.submitting = true
				return m, tea.Batch(m.spinner.Tick, m.attachVolume(vol))
			}
		default:
			s := msg.String()
			if s == "backspace" {
				if len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
					m.cursor = 0
					m.scrollOff = 0
					m.applyFilter()
				}
			} else if len(s) == 1 && s[0] >= 32 && s[0] < 127 {
				m.filter += s
				m.cursor = 0
				m.scrollOff = 0
				m.applyFilter()
			}
		}
	}
	return m, nil
}

func (m *Model) applyFilter() {
	if m.filter == "" {
		m.filtered = m.volumes
		return
	}
	q := strings.ToLower(m.filter)
	m.filtered = nil
	for _, vol := range m.volumes {
		if strings.Contains(strings.ToLower(vol.Name), q) ||
			strings.Contains(strings.ToLower(vol.ID), q) {
			m.filtered = append(m.filtered, vol)
		}
	}
}

func (m *Model) ensureVisible() {
	th := m.listHeight()
	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+th {
		m.scrollOff = m.cursor - th + 1
	}
}

func (m Model) listHeight() int {
	h := m.height - 14
	if h < 3 {
		h = 3
	}
	return h
}

// View renders the volume picker modal.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Attach Volume to " + m.serverName)

	var body string
	if m.loading {
		body = m.spinner.View() + " Loading volumes..."
	} else if m.submitting {
		body = m.spinner.View() + " Attaching..."
	} else if m.err != "" {
		body = lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.err)
		body += "\n\n" + shared.StyleHelp.Render("esc to close")
	} else if len(m.filtered) == 0 {
		body = shared.StyleHelp.Render("No available volumes found")
		body += "\n\n" + shared.StyleHelp.Render("esc to close")
	} else {
		var lines []string
		th := m.listHeight()
		end := m.scrollOff + th
		if end > len(m.filtered) {
			end = len(m.filtered)
		}

		for i := m.scrollOff; i < end; i++ {
			vol := m.filtered[i]
			cursor := "  "
			if i == m.cursor {
				cursor = "▸ "
			}
			style := lipgloss.NewStyle().Foreground(shared.ColorFg)
			if i == m.cursor {
				style = style.Foreground(shared.ColorHighlight).Bold(true)
			}
			mutedStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)

			name := vol.Name
			if name == "" {
				name = vol.ID[:12]
			}
			detail := mutedStyle.Render(fmt.Sprintf(" %dGB", vol.Size))
			if vol.VolumeType != "" {
				detail += mutedStyle.Render(" • " + vol.VolumeType)
			}
			line := cursor + style.Render(name) + detail
			lines = append(lines, line)
		}
		body = strings.Join(lines, "\n")

		filterHint := ""
		if m.filter != "" {
			filterHint = fmt.Sprintf("\n\nFilter: %s", m.filter)
		}
		body += filterHint
		body += "\n\n" + shared.StyleHelp.Render("↑↓ navigate • enter select • type to filter • esc cancel")
	}

	content := title + "\n\n" + body
	modalWidth := 60
	if m.width > 0 && m.width < 70 {
		modalWidth = m.width - 6
	}
	box := shared.StyleModal.Width(modalWidth).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m Model) fetchVolumes() tea.Cmd {
	client := m.blockClient
	return func() tea.Msg {
		vols, err := volume.ListVolumes(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		var available []volume.Volume
		for _, v := range vols {
			if v.Status == "available" && v.AttachedServerID == "" {
				available = append(available, v)
			}
		}
		return volumesLoadedMsg{volumes: available}
	}
}

func (m Model) attachVolume(vol volume.Volume) tea.Cmd {
	client := m.computeClient
	serverID := m.serverID
	serverName := m.serverName
	volumeID := vol.ID
	volumeName := vol.Name
	if volumeName == "" {
		volumeName = vol.ID[:12]
	}
	return func() tea.Msg {
		err := volume.AttachVolume(context.Background(), client, serverID, volumeID)
		if err != nil {
			return attachErrMsg{err: err}
		}
		return attachDoneMsg{serverName: serverName, volumeName: volumeName}
	}
}
