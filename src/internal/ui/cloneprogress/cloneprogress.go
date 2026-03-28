package cloneprogress

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/volume"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	bsvolumes "github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
)

// VolumeOp tracks the state of a single volume clone+attach operation.
type VolumeOp struct {
	SourceVolID string
	SourceName  string
	CloneName   string
	CloneVolID  string // set after creation
	Status      string // pending, creating, available, attaching, done, error
	Err         error
}

// AllCompleteMsg is sent when all volume operations finish successfully.
type AllCompleteMsg struct{}

// RollbackCompleteMsg is sent after rollback finishes.
type RollbackCompleteMsg struct {
	Errors []error
}

type volumeCreatedMsg struct {
	idx    int
	volID  string
	err    error
}

type volumeStatusMsg struct {
	idx    int
	status string
	err    error
}

type volumeAttachedMsg struct {
	idx int
	err error
}

type rollbackDoneMsg struct {
	errors []error
}

type pollTickMsg struct{}

// Model is the clone progress modal.
type Model struct {
	Active        bool
	computeClient *gophercloud.ServiceClient
	volumeClient  *gophercloud.ServiceClient
	serverID      string
	serverName    string
	volumes       []VolumeOp
	spinner       spinner.Model
	running       bool // operations still in progress
	polling       bool // poll tick is scheduled
	failed        bool
	rollingBack   bool
	width         int
	height        int
}

// New creates a clone progress model.
func New(computeClient, volumeClient *gophercloud.ServiceClient, serverID, serverName string, ops []VolumeOp) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		Active:        true,
		computeClient: computeClient,
		volumeClient:  volumeClient,
		serverID:      serverID,
		serverName:    serverName,
		volumes:       ops,
		spinner:       s,
		running:       true,
	}
}

// Init kicks off volume creation for all volumes in parallel.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick}
	for i, op := range m.volumes {
		cmds = append(cmds, m.createVolume(i, op))
	}
	return tea.Batch(cmds...)
}

// Running returns true if operations are still in progress.
func (m Model) Running() bool {
	return m.running
}

// ServerName returns the name of the cloned server.
func (m Model) ServerName() string {
	return m.serverName
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, shared.Keys.Back) && m.Active {
			m.Active = false
			return m, nil
		}
		return m, nil

	case spinner.TickMsg:
		if m.running {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case volumeCreatedMsg:
		if msg.err != nil {
			m.volumes[msg.idx].Status = "error"
			m.volumes[msg.idx].Err = msg.err
			return m.startRollback()
		}
		m.volumes[msg.idx].CloneVolID = msg.volID
		m.volumes[msg.idx].Status = "creating"
		// Only schedule poll if not already polling
		if !m.polling {
			m.polling = true
			return m, m.schedulePoll()
		}
		return m, nil

	case pollTickMsg:
		if m.failed || m.rollingBack {
			m.polling = false
			return m, nil
		}
		cmd := m.pollVolumes()
		if cmd == nil {
			m.polling = false
		}
		return m, cmd

	case volumeStatusMsg:
		if msg.err != nil {
			m.volumes[msg.idx].Status = "error"
			m.volumes[msg.idx].Err = msg.err
			return m.startRollback()
		}
		if msg.status == "available" {
			m.volumes[msg.idx].Status = "attaching"
			cmds := []tea.Cmd{m.attachVolume(msg.idx)}
			// Ensure polling continues for remaining creating volumes
			if m.hasCreatingVolumes() && !m.polling {
				m.polling = true
				cmds = append(cmds, m.schedulePoll())
			}
			return m, tea.Batch(cmds...)
		}
		if msg.status == "error" {
			m.volumes[msg.idx].Status = "error"
			m.volumes[msg.idx].Err = fmt.Errorf("volume entered error state")
			return m.startRollback()
		}
		// Still creating — schedule next poll cycle
		if m.hasCreatingVolumes() && !m.polling {
			m.polling = true
			return m, m.schedulePoll()
		}
		return m, nil

	case volumeAttachedMsg:
		if msg.err != nil {
			m.volumes[msg.idx].Status = "error"
			m.volumes[msg.idx].Err = msg.err
			return m.startRollback()
		}
		m.volumes[msg.idx].Status = "done"
		if m.allDone() {
			m.running = false
			return m, func() tea.Msg { return AllCompleteMsg{} }
		}
		return m, nil

	case rollbackDoneMsg:
		m.running = false
		m.rollingBack = false
		return m, func() tea.Msg {
			return RollbackCompleteMsg{Errors: msg.errors}
		}
	}
	return m, nil
}

// View renders the progress modal.
func (m Model) View() string {
	if !m.Active {
		return ""
	}

	var b strings.Builder

	title := "Clone Volumes"
	if m.rollingBack {
		title = "Rolling Back"
	}
	b.WriteString(shared.StyleModalTitle.Render(title) + "\n")
	b.WriteString(shared.StyleHelp.Render(fmt.Sprintf("Server: %s", m.serverName)) + "\n\n")

	for _, op := range m.volumes {
		icon := "○"
		style := lipgloss.NewStyle().Foreground(shared.ColorMuted)
		statusText := op.Status

		switch op.Status {
		case "pending":
			icon = "○"
			statusText = "pending"
		case "creating":
			icon = m.spinner.View()
			style = lipgloss.NewStyle().Foreground(shared.ColorWarning)
			statusText = "creating..."
		case "available":
			icon = "▲"
			style = lipgloss.NewStyle().Foreground(shared.ColorWarning)
			statusText = "ready"
		case "attaching":
			icon = m.spinner.View()
			style = lipgloss.NewStyle().Foreground(shared.ColorCyan)
			statusText = "attaching..."
		case "done":
			icon = "●"
			style = lipgloss.NewStyle().Foreground(shared.ColorSuccess)
			statusText = "attached"
		case "error":
			icon = "✘"
			style = lipgloss.NewStyle().Foreground(shared.ColorError)
			if op.Err != nil {
				statusText = op.Err.Error()
			}
		}

		name := lipgloss.NewStyle().Foreground(shared.ColorFg).Width(30).Render(op.CloneName)
		b.WriteString(fmt.Sprintf("  %s %s %s\n", icon, name, style.Render(statusText)))
	}

	b.WriteString("\n")
	b.WriteString(shared.StyleHelp.Render("  esc dismiss (operations continue in background)") + "\n")

	box := shared.StyleModal.Width(60).Render(b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m Model) createVolume(idx int, op VolumeOp) tea.Cmd {
	client := m.volumeClient
	return func() tea.Msg {
		// Get source volume to determine size
		src, err := volume.GetVolume(context.Background(), client, op.SourceVolID)
		if err != nil {
			return volumeCreatedMsg{idx: idx, err: fmt.Errorf("fetching source volume: %w", err)}
		}
		opts := bsvolumes.CreateOpts{
			Name:        op.CloneName,
			Size:        src.Size,
			SourceVolID: op.SourceVolID,
			VolumeType:  src.VolumeType,
		}
		vol, err := volume.CreateVolume(context.Background(), client, opts)
		if err != nil {
			return volumeCreatedMsg{idx: idx, err: err}
		}
		return volumeCreatedMsg{idx: idx, volID: vol.ID}
	}
}

func (m Model) schedulePoll() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return pollTickMsg{}
	})
}

func (m Model) pollVolumes() tea.Cmd {
	// Poll all volumes in "creating" state
	var cmds []tea.Cmd
	for i, op := range m.volumes {
		if op.Status == "creating" && op.CloneVolID != "" {
			idx := i
			volID := op.CloneVolID
			client := m.volumeClient
			cmds = append(cmds, func() tea.Msg {
				vol, err := volume.GetVolume(context.Background(), client, volID)
				if err != nil {
					return volumeStatusMsg{idx: idx, err: err}
				}
				return volumeStatusMsg{idx: idx, status: vol.Status}
			})
		}
	}
	if len(cmds) > 0 {
		// Also schedule the next poll tick
		cmds = append(cmds, m.schedulePoll())
		return tea.Batch(cmds...)
	}
	// If no volumes are in "creating" state but some are pending,
	// keep polling (they may transition soon)
	for _, op := range m.volumes {
		if op.Status == "pending" {
			return m.schedulePoll()
		}
	}
	return nil
}

func (m Model) hasCreatingVolumes() bool {
	for _, op := range m.volumes {
		if op.Status == "creating" {
			return true
		}
	}
	return false
}

func (m Model) attachVolume(idx int) tea.Cmd {
	volID := m.volumes[idx].CloneVolID
	serverID := m.serverID
	computeClient := m.computeClient
	return func() tea.Msg {
		err := volume.AttachVolume(context.Background(), computeClient, serverID, volID)
		return volumeAttachedMsg{idx: idx, err: err}
	}
}

func (m Model) allDone() bool {
	for _, op := range m.volumes {
		if op.Status != "done" {
			return false
		}
	}
	return true
}

func (m Model) startRollback() (Model, tea.Cmd) {
	if m.rollingBack {
		return m, nil
	}
	m.failed = true
	m.rollingBack = true

	// Collect volume IDs to delete and server to delete
	var volIDs []string
	for _, op := range m.volumes {
		if op.CloneVolID != "" {
			volIDs = append(volIDs, op.CloneVolID)
		}
	}

	volumeClient := m.volumeClient
	computeClient := m.computeClient
	serverID := m.serverID

	return m, func() tea.Msg {
		var errs []error
		// Delete cloned volumes
		for _, vid := range volIDs {
			if err := volume.DeleteVolume(context.Background(), volumeClient, vid); err != nil {
				errs = append(errs, fmt.Errorf("delete volume %s: %w", vid, err))
			}
		}
		// Delete the cloned server
		if err := compute.DeleteServer(context.Background(), computeClient, serverID); err != nil {
			errs = append(errs, fmt.Errorf("delete server %s: %w", serverID, err))
		}
		return rollbackDoneMsg{errors: errs}
	}
}
