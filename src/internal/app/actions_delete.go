package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/modal"
	"github.com/larkly/lazystack/internal/volume"
)

// Detach-wait tuning for delete-with-volumes. The wait is bounded; volumes
// whose detachment is never confirmed are reported, not deleted.
var (
	volumeDetachPollAttempts = 10
	volumeDetachPollInterval = 3 * time.Second
	// sleepCtx waits for d or until ctx is done. Tests replace it so the
	// retry and exhaustion paths run without real delays.
	sleepCtx = func(ctx context.Context, d time.Duration) error {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}
)

// volumeCleanupFailure records a volume that was not deleted along with its
// server, and why.
type volumeCleanupFailure struct {
	volumeID string
	err      error
}

// serverDeletedMsg reports a successful server deletion together with the
// outcome of the optional attached-volume cleanup.
type serverDeletedMsg struct {
	id, name       string
	volumesDeleted []string
	volumeFailures []volumeCleanupFailure
}

// waitVolumesDetached polls until every volume reports "available" or the
// bounded wait is exhausted. It returns, for each volume whose detachment
// could not be confirmed, the reason (last lookup error or last status).
// A failed lookup never counts as detached.
func waitVolumesDetached(ctx context.Context, lookup func(context.Context, string) (*volume.Volume, error), volIDs []string) map[string]error {
	pending := make(map[string]error, len(volIDs))
	for _, vid := range volIDs {
		pending[vid] = errors.New("detachment not confirmed")
	}
	for attempt := 0; attempt < volumeDetachPollAttempts && len(pending) > 0; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, volumeDetachPollInterval); err != nil {
				break
			}
		}
		for _, vid := range volIDs {
			if _, ok := pending[vid]; !ok {
				continue
			}
			v, err := lookup(ctx, vid)
			switch {
			case err != nil:
				pending[vid] = fmt.Errorf("status lookup failed: %w", err)
			case v.Status != "available":
				pending[vid] = fmt.Errorf("still %s after waiting for detachment", v.Status)
			default:
				delete(pending, vid)
			}
		}
	}
	return pending
}

// deleteServerCmd deletes a server and, when requested, its attached
// volumes. Volumes are only deleted once they are confirmed detached; any
// volume that could not be cleaned up is reported with its ID and error.
func (m Model) deleteServerCmd(action modal.ConfirmAction) tea.Cmd {
	client := m.client.Compute
	bsClient := m.client.BlockStorage
	deleteVols := action.DeleteVolumes && bsClient != nil
	volIDs := action.VolumeIDs
	return func() tea.Msg {
		ctx, cancel := actionCtxLong()
		defer cancel()
		shared.Debugf("[action] deleting server %s", action.Name)
		detachErrs := map[string]error{}
		var unsafe map[string]error
		if deleteVols {
			for _, vid := range volIDs {
				if err := volume.DetachVolume(ctx, client, action.ServerID, vid); err != nil {
					detachErrs[vid] = err
				}
			}
			unsafe = waitVolumesDetached(ctx, func(ctx context.Context, id string) (*volume.Volume, error) {
				return volume.GetVolume(ctx, bsClient, id)
			}, volIDs)
		}

		if err := compute.DeleteServer(ctx, client, action.ServerID); err != nil {
			shared.Debugf("[action] delete server %s failed: %s", action.Name, err)
			m.logAudit(audit.ActionDelete, "server", action.ServerID, action.Name, "error", err.Error())
			return shared.ServerActionErrMsg{Action: "Delete", Name: action.Name, Err: err}
		}
		shared.Debugf("[action] deleted server %s", action.Name)
		m.logAudit(audit.ActionDelete, "server", action.ServerID, action.Name, "success", "")

		msg := serverDeletedMsg{id: action.ServerID, name: action.Name}
		if !deleteVols {
			return msg
		}
		for _, vid := range volIDs {
			var steps []string
			if err := detachErrs[vid]; err != nil {
				steps = append(steps, fmt.Sprintf("detach: %v", err))
			}
			if reason, ok := unsafe[vid]; ok {
				steps = append(steps, fmt.Sprintf("not deleted: %v", reason))
				err := errors.New(strings.Join(steps, "; "))
				shared.Debugf("[action] skipping delete of volume %s: %s", vid, err)
				m.logAudit(audit.ActionDelete, "volume", vid, vid, "skipped", err.Error())
				msg.volumeFailures = append(msg.volumeFailures, volumeCleanupFailure{volumeID: vid, err: err})
				continue
			}
			if err := volume.DeleteVolume(ctx, bsClient, vid); err != nil {
				steps = append(steps, fmt.Sprintf("delete: %v", err))
				err := errors.New(strings.Join(steps, "; "))
				shared.Debugf("[action] delete volume %s failed: %s", vid, err)
				m.logAudit(audit.ActionDelete, "volume", vid, vid, "error", err.Error())
				msg.volumeFailures = append(msg.volumeFailures, volumeCleanupFailure{volumeID: vid, err: err})
				continue
			}
			m.logAudit(audit.ActionDelete, "volume", vid, vid, "success", "")
			msg.volumesDeleted = append(msg.volumesDeleted, vid)
		}
		return msg
	}
}

// handleServerDeleted leaves the detail view of the deleted server, reports
// the outcome and lists every volume that could not be cleaned up.
func (m Model) handleServerDeleted(msg serverDeletedMsg) (Model, tea.Cmd) {
	m.statusBar.Error = ""
	m.serverResize.Active = false
	if m.view == viewServerDetail && m.serverDetail.ServerID() == msg.id {
		m.returnToView = 0
		m.view = viewServerList
		m.statusBar.CurrentView = "serverlist"
		m.statusBar.Hint = m.serverList.Hints()
	}
	refresh := func() tea.Msg { return shared.RefreshServersMsg{} }
	if len(msg.volumeFailures) == 0 {
		m.statusBar.StickyHint = fmt.Sprintf("✓ Delete %s", msg.name)
		return m, refresh
	}
	m.statusBar.StickyHint = fmt.Sprintf("⚠ Deleted %s; %d volume(s) not cleaned up", msg.name, len(msg.volumeFailures))
	lines := make([]string, len(msg.volumeFailures))
	for i, f := range msg.volumeFailures {
		lines[i] = fmt.Sprintf("volume %s: %v", f.volumeID, f.err)
	}
	m.errModal = modal.NewError(
		fmt.Sprintf("Server %s deleted, but %d volume(s) were not deleted", msg.name, len(msg.volumeFailures)),
		errors.New(strings.Join(lines, "\n")))
	m.errModal.SetSize(m.width, m.height)
	m.activeModal = modalError
	return m, refresh
}
