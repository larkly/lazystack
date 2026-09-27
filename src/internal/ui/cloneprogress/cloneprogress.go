package cloneprogress

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	bsvolumes "github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/volume"
)

// pollInterval is how often volume and server status are polled. It is a
// variable so tests can shorten it.
var pollInterval = 3 * time.Second

// maxPollErrors is how many consecutive failed status polls of one volume
// (or of the server) are tolerated before the clone is treated as failed.
const maxPollErrors = 5

// Rollback waits for the deleted server to disappear and for each cloned
// volume to become deletable. Variables so tests can shorten them.
var (
	rollbackPollInterval = 2 * time.Second
	rollbackWaitTimeout  = 3 * time.Minute
)

// VolumeOp tracks the state of a single volume clone+attach operation.
type VolumeOp struct {
	SourceVolID string
	SourceName  string
	CloneName   string
	CloneVolID  string // set after creation
	Status      string // pending, creating, available, attaching, done, error
	Err         error
	pollErrs    int // consecutive failed status polls
}

// AllCompleteMsg is sent when all volume operations finish successfully.
type AllCompleteMsg struct {
	Op         uint64 // ID of the clone operation that completed
	ServerName string
}

// RollbackCompleteMsg is sent after rollback finishes.
type RollbackCompleteMsg struct {
	Op       uint64   // ID of the clone operation that was rolled back
	Cause    error    // the original error that triggered rollback
	Errors   []error  // errors during cleanup
	Leftover []string // resources that could not be removed
}

// Every internal message carries the ID of the clone operation that issued
// it, so a late reply from a dismissed clone can never mutate another one.

type volumeCreatedMsg struct {
	op    uint64
	idx   int
	volID string
	err   error
}

type volumeStatusMsg struct {
	op     uint64
	idx    int
	status string
	err    error
}

type volumeAttachedMsg struct {
	op  uint64
	idx int
	err error
}

type rollbackDoneMsg struct {
	op       uint64
	cause    error
	errors   []error
	leftover []string
}

type serverReadyMsg struct {
	op     uint64
	status string
	err    error
}

type pollTickMsg struct {
	op uint64
}

// lastOpID hands out a unique ID to every clone operation.
var lastOpID atomic.Uint64

// OpID returns the clone operation ID carried by msg, and false when msg
// is not a clone progress message.
func OpID(msg tea.Msg) (uint64, bool) {
	switch msg := msg.(type) {
	case AllCompleteMsg:
		return msg.Op, true
	case RollbackCompleteMsg:
		return msg.Op, true
	case volumeCreatedMsg:
		return msg.op, true
	case volumeStatusMsg:
		return msg.op, true
	case volumeAttachedMsg:
		return msg.op, true
	case rollbackDoneMsg:
		return msg.op, true
	case serverReadyMsg:
		return msg.op, true
	case pollTickMsg:
		return msg.op, true
	case volumeNamesResolvedMsg:
		return msg.op, true
	}
	return 0, false
}

// Model is the clone progress modal.
type Model struct {
	Active        bool
	op            uint64
	computeClient *gophercloud.ServiceClient
	volumeClient  *gophercloud.ServiceClient
	serverID      string
	serverName    string
	serverReady   bool // server has reached ACTIVE state
	volumes       []VolumeOp
	spinner       spinner.Model
	running       bool // operations still in progress
	polling       bool // poll tick is scheduled
	failed        bool
	rollingBack   bool
	cause         error // what made the clone fail, if not a volume error
	width         int
	height        int
	pendingAttach []int // volume indices waiting for server to be ready
	serverErrs    int   // consecutive failed server status polls

	createsIssued    bool // volume create requests have been sent
	rollbackLaunched bool // cleanup command has been started
}

// New creates a clone progress model.
func New(computeClient, volumeClient *gophercloud.ServiceClient, serverID, serverName string, ops []VolumeOp) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		Active:        true,
		op:            lastOpID.Add(1),
		computeClient: computeClient,
		volumeClient:  volumeClient,
		serverID:      serverID,
		serverName:    serverName,
		volumes:       ops,
		spinner:       s,
		running:       true,
	}
}

type volumeNamesResolvedMsg struct {
	op  uint64
	ops []VolumeOp
}

// Init resolves volume names async, then kicks off creation.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[cloneprogress] clone start server=%q volumes=%d", m.serverName, len(m.volumes))
	client := m.volumeClient
	ops := m.volumes
	op := m.op
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		// Fetch existing volume names for display and dedup
		existingNames := make(map[string]bool)
		nameMap := make(map[string]string)
		if client != nil {
			vols, err := volume.ListVolumes(context.Background(), client)
			if err == nil {
				for _, v := range vols {
					existingNames[v.Name] = true
					nameMap[v.ID] = v.Name
				}
			}
		}

		resolved := make([]VolumeOp, len(ops))
		for i, op := range ops {
			resolved[i] = op
			if name, ok := nameMap[op.SourceVolID]; ok {
				resolved[i].SourceName = name
				resolved[i].CloneName = shared.DeduplicateName(name, existingNames)
			} else {
				resolved[i].CloneName = shared.DeduplicateName(op.SourceVolID, existingNames)
			}
			existingNames[resolved[i].CloneName] = true
		}
		return volumeNamesResolvedMsg{op: op, ops: resolved}
	})
}

// ID returns the unique ID of this clone operation.
func (m Model) ID() uint64 {
	return m.op
}

// validIdx reports whether idx addresses one of this clone's volumes.
func (m Model) validIdx(idx int) bool {
	return idx >= 0 && idx < len(m.volumes)
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
	if op, ok := OpID(msg); ok && op != m.op {
		shared.Debugf("[cloneprogress] ignoring message for clone op %d (this is %d)", op, m.op)
		return m, nil
	}
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

	case volumeNamesResolvedMsg:
		if !m.running || m.failed || m.createsIssued || len(msg.ops) != len(m.volumes) {
			return m, nil
		}
		m.volumes = msg.ops
		m.createsIssued = true
		cmds := make([]tea.Cmd, len(m.volumes))
		for i, op := range m.volumes {
			cmds[i] = m.createVolume(i, op)
		}
		return m, tea.Batch(cmds...)

	case volumeCreatedMsg:
		if !m.validIdx(msg.idx) || !m.running || m.volumes[msg.idx].Status != "pending" {
			return m, nil
		}
		if msg.err != nil {
			m.volumes[msg.idx].Status = "error"
			m.volumes[msg.idx].Err = msg.err
			shared.Debugf("[cloneprogress] error creating volume idx=%d: %v", msg.idx, msg.err)
			if m.failed {
				return m.launchRollback()
			}
			return m.startRollback(nil)
		}
		m.volumes[msg.idx].CloneVolID = msg.volID
		m.volumes[msg.idx].Status = "creating"
		shared.Debugf("[cloneprogress] volume created idx=%d volID=%s", msg.idx, msg.volID)
		if m.failed {
			// A create that was in flight when the clone failed: the new
			// volume is ours, so rollback must wait for it and remove it.
			return m.launchRollback()
		}
		return m, m.ensurePolling()

	case pollTickMsg:
		// This tick is consumed; ensurePolling below schedules the next one
		// only while some step still waits on a status poll.
		m.polling = false
		if m.failed || m.rollingBack || !m.running {
			return m, nil
		}
		cmds := m.pollVolumes()
		// Also check server readiness if we have pending attaches
		if !m.serverReady && len(m.pendingAttach) > 0 {
			cmds = append(cmds, m.checkServerReady())
		}
		cmds = append(cmds, m.ensurePolling())
		return m, tea.Batch(cmds...)

	case volumeStatusMsg:
		if !m.validIdx(msg.idx) || !m.running || m.failed {
			return m, nil
		}
		// Only volumes still being created are polled; a late duplicate
		// reply must not attach a volume twice.
		if m.volumes[msg.idx].Status != "creating" {
			return m, nil
		}
		if msg.err != nil {
			// Tolerate transient API errors; the poll chain retries.
			m.volumes[msg.idx].pollErrs++
			if m.volumes[msg.idx].pollErrs < maxPollErrors {
				shared.Debugf("[cloneprogress] volume poll idx=%d failed (%d/%d): %v", msg.idx, m.volumes[msg.idx].pollErrs, maxPollErrors, msg.err)
				return m, nil
			}
			m.volumes[msg.idx].Status = "error"
			m.volumes[msg.idx].Err = msg.err
			return m.startRollback(nil)
		}
		m.volumes[msg.idx].pollErrs = 0
		if msg.status == "available" {
			m.volumes[msg.idx].Status = "available"
			return m.tryAttach(msg.idx)
		}
		if msg.status == "error" {
			m.volumes[msg.idx].Status = "error"
			m.volumes[msg.idx].Err = fmt.Errorf("volume entered error state")
			return m.startRollback(nil)
		}
		// Still creating — the running poll chain checks it again.
		return m, nil

	case serverReadyMsg:
		if !m.running || m.failed || m.serverReady {
			return m, nil
		}
		if msg.err != nil {
			// Tolerate transient API errors; the running poll chain retries.
			m.serverErrs++
			if m.serverErrs < maxPollErrors {
				return m, nil
			}
			return m.startRollback(fmt.Errorf("checking cloned server %s: %w", m.serverName, msg.err))
		}
		m.serverErrs = 0
		if msg.status == "ERROR" {
			return m.startRollback(fmt.Errorf("cloned server %s entered ERROR state", m.serverName))
		}
		if msg.status == "ACTIVE" {
			m.serverReady = true
			// Attach all volumes that were waiting
			var cmds []tea.Cmd
			for _, idx := range m.pendingAttach {
				if !m.validIdx(idx) {
					continue
				}
				m.volumes[idx].Status = "attaching"
				cmds = append(cmds, m.attachVolume(idx))
			}
			m.pendingAttach = nil
			return m, tea.Batch(cmds...)
		}
		// Not ready yet — the running poll chain checks it again. Never
		// schedule a tick here: that would start a second chain.
		return m, nil

	case volumeAttachedMsg:
		if !m.validIdx(msg.idx) || !m.running || m.failed {
			return m, nil
		}
		if msg.err != nil {
			m.volumes[msg.idx].Status = "error"
			m.volumes[msg.idx].Err = msg.err
			shared.Debugf("[cloneprogress] error attaching volume idx=%d: %v", msg.idx, msg.err)
			return m.startRollback(nil)
		}
		m.volumes[msg.idx].Status = "done"
		shared.Debugf("[cloneprogress] volume attached idx=%d", msg.idx)
		if m.allDone() {
			m.running = false
			shared.Debugf("[cloneprogress] all volumes cloned and attached successfully")
			op, name := m.op, m.serverName
			return m, func() tea.Msg { return AllCompleteMsg{Op: op, ServerName: name} }
		}
		return m, nil

	case rollbackDoneMsg:
		if !m.rollingBack {
			return m, nil
		}
		m.running = false
		m.rollingBack = false
		op := m.op
		return m, func() tea.Msg {
			return RollbackCompleteMsg{Op: op, Cause: msg.cause, Errors: msg.errors, Leftover: msg.leftover}
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
			icon = m.spinner.View()
			style = lipgloss.NewStyle().Foreground(shared.ColorWarning)
			statusText = "ready, waiting..."
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

		name := lipgloss.NewStyle().Foreground(shared.ColorFg).Width(25).Render(op.CloneName)
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
	opID := m.op
	return func() tea.Msg {
		// Get source volume to determine size
		src, err := volume.GetVolume(context.Background(), client, op.SourceVolID)
		if err != nil {
			return volumeCreatedMsg{op: opID, idx: idx, err: fmt.Errorf("fetching source volume: %w", err)}
		}
		opts := bsvolumes.CreateOpts{
			Name:        op.CloneName,
			Size:        src.Size,
			SourceVolID: op.SourceVolID,
			VolumeType:  src.VolumeType,
		}
		vol, err := volume.CreateVolume(context.Background(), client, opts)
		if err != nil {
			return volumeCreatedMsg{op: opID, idx: idx, err: err}
		}
		return volumeCreatedMsg{op: opID, idx: idx, volID: vol.ID}
	}
}

func (m Model) schedulePoll() tea.Cmd {
	op := m.op
	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		return pollTickMsg{op: op}
	})
}

// needsPoll reports whether any step is waiting on a status poll.
func (m Model) needsPoll() bool {
	return m.hasCreatingVolumes() || (!m.serverReady && len(m.pendingAttach) > 0)
}

// ensurePolling schedules the next poll tick unless one is already pending
// or nothing needs polling. It is the only place poll ticks are scheduled,
// so each clone has at most one poll chain.
func (m *Model) ensurePolling() tea.Cmd {
	if m.polling || m.failed || !m.running || !m.needsPoll() {
		return nil
	}
	m.polling = true
	return m.schedulePoll()
}

// pollVolumes returns one status request per volume still being created.
func (m Model) pollVolumes() []tea.Cmd {
	var cmds []tea.Cmd
	for i, op := range m.volumes {
		if op.Status == "creating" && op.CloneVolID != "" {
			idx := i
			volID := op.CloneVolID
			client := m.volumeClient
			op := m.op
			cmds = append(cmds, func() tea.Msg {
				vol, err := volume.GetVolume(context.Background(), client, volID)
				if err != nil {
					return volumeStatusMsg{op: op, idx: idx, err: err}
				}
				return volumeStatusMsg{op: op, idx: idx, status: vol.Status}
			})
		}
	}
	return cmds
}

func (m Model) hasCreatingVolumes() bool {
	for _, op := range m.volumes {
		if op.Status == "creating" {
			return true
		}
	}
	return false
}

func (m Model) tryAttach(idx int) (Model, tea.Cmd) {
	if m.serverReady {
		m.volumes[idx].Status = "attaching"
		return m, m.attachVolume(idx)
	}
	// Server not ready yet — queue this volume and check server status
	m.pendingAttach = append(m.pendingAttach, idx)
	cmds := []tea.Cmd{m.ensurePolling()}
	if len(m.pendingAttach) == 1 {
		// First pending — check the server right away
		cmds = append(cmds, m.checkServerReady())
	}
	return m, tea.Batch(cmds...)
}

func (m Model) checkServerReady() tea.Cmd {
	client := m.computeClient
	id := m.serverID
	op := m.op
	return func() tea.Msg {
		srv, err := compute.GetServer(context.Background(), client, id)
		if err != nil {
			return serverReadyMsg{op: op, err: err}
		}
		return serverReadyMsg{op: op, status: srv.Status}
	}
}

func (m Model) attachVolume(idx int) tea.Cmd {
	volID := m.volumes[idx].CloneVolID
	serverID := m.serverID
	computeClient := m.computeClient
	op := m.op
	return func() tea.Msg {
		_, err := volume.AttachVolume(context.Background(), computeClient, serverID, volID)
		return volumeAttachedMsg{op: op, idx: idx, err: err}
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

// startRollback marks the clone failed and cleans up the resources it owns.
// It runs at most once per operation. cause overrides the volume error
// reported as the reason, when the failure did not come from a volume.
func (m Model) startRollback(cause error) (Model, tea.Cmd) {
	if m.failed || m.rollingBack {
		return m, nil
	}
	m.failed = true
	m.rollingBack = true
	m.cause = cause
	return m.launchRollback()
}

// createsInFlight reports whether a volume create request has not
// answered yet; its volume would be missed by a cleanup started now.
func (m Model) createsInFlight() bool {
	if !m.createsIssued {
		return false
	}
	for _, op := range m.volumes {
		if op.Status == "pending" {
			return true
		}
	}
	return false
}

// ownedVolumeIDs returns the IDs of the volumes this clone created.
func (m Model) ownedVolumeIDs() []string {
	var ids []string
	for _, op := range m.volumes {
		if op.CloneVolID != "" {
			ids = append(ids, op.CloneVolID)
		}
	}
	return ids
}

// launchRollback starts the cleanup command once no create request is in
// flight, so every volume this clone created is known.
func (m Model) launchRollback() (Model, tea.Cmd) {
	if !m.rollingBack || m.rollbackLaunched || m.createsInFlight() {
		return m, nil
	}
	m.rollbackLaunched = true

	cause := m.cause
	if cause == nil {
		for _, op := range m.volumes {
			if op.Err != nil {
				cause = op.Err
				break
			}
		}
	}
	volIDs := m.ownedVolumeIDs()
	volumeClient := m.volumeClient
	computeClient := m.computeClient
	serverID := m.serverID
	op := m.op

	return m, func() tea.Msg {
		errs, leftover := rollbackResources(computeClient, volumeClient, serverID, volIDs)
		return rollbackDoneMsg{op: op, cause: cause, errors: errs, leftover: leftover}
	}
}

// rollbackResources deletes the cloned server, waits for it to be gone,
// then deletes each cloned volume once Cinder allows it. Volumes attached
// to the server cannot be deleted before the server is removed. It returns
// the cleanup errors and the resources left behind.
func rollbackResources(computeClient, volumeClient *gophercloud.ServiceClient, serverID string, volIDs []string) ([]error, []string) {
	ctx, cancel := context.WithTimeout(context.Background(), rollbackWaitTimeout)
	defer cancel()
	var errs []error
	var leftover []string

	serverGone := false
	if err := compute.DeleteServer(ctx, computeClient, serverID); err != nil && !isNotFound(err) {
		errs = append(errs, fmt.Errorf("delete server %s: %w", serverID, err))
	} else {
		serverGone = waitUntil(ctx, func() bool {
			_, err := compute.GetServer(ctx, computeClient, serverID)
			return isNotFound(err)
		})
		if !serverGone {
			errs = append(errs, fmt.Errorf("server %s still present after delete", serverID))
		}
	}
	if !serverGone {
		leftover = append(leftover, "server "+serverID)
	}

	for _, vid := range volIDs {
		if err := deleteVolumeWhenFree(ctx, volumeClient, vid); err != nil {
			errs = append(errs, err)
			leftover = append(leftover, "volume "+vid)
		}
	}
	return errs, leftover
}

// deleteVolumeWhenFree waits until the volume is in a deletable state and
// deletes it. A volume that is already gone counts as deleted.
func deleteVolumeWhenFree(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	var last string
	done := waitUntil(ctx, func() bool {
		v, err := volume.GetVolume(ctx, client, id)
		switch {
		case isNotFound(err):
			last = "deleted"
			return true
		case err != nil:
			last = err.Error()
			return false
		}
		last = v.Status
		switch v.Status {
		case "available", "error", "error_restoring", "error_extending", "error_managing":
			return true
		}
		return false
	})
	if !done {
		return fmt.Errorf("delete volume %s: not deletable before timeout (last: %s)", id, last)
	}
	if last == "deleted" {
		return nil
	}
	if err := volume.DeleteVolume(ctx, client, id); err != nil && !isNotFound(err) {
		return fmt.Errorf("delete volume %s: %w", id, err)
	}
	return nil
}

// waitUntil polls cond until it returns true or ctx expires.
func waitUntil(ctx context.Context, cond func() bool) bool {
	for {
		if cond() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(rollbackPollInterval):
		}
	}
}

func isNotFound(err error) bool {
	return err != nil && gophercloud.ResponseCodeIs(err, http.StatusNotFound)
}
