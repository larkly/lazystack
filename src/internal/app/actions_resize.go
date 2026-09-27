package app

import (
	"context"
	"fmt"

	"charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/modal"
)

// resizeStep describes confirming or reverting a pending resize.
type resizeStep struct {
	label   string // "Confirm resize" / "Revert resize"
	pending string // optimistic detail label while the request runs
	verb    string // for the in-progress hint
	audit   audit.ActionType
	call    func(context.Context, *gophercloud.ServiceClient, string) error
}

var (
	confirmResizeStep = resizeStep{"Confirm resize", "Resize confirmed", "Confirming", audit.ActionConfirmResize, compute.ConfirmResize}
	revertResizeStep  = resizeStep{"Revert resize", "Resize reverted", "Reverting", audit.ActionRevertResize, compute.RevertResize}
)

// pendingResize remembers which operation set the optimistic resize label
// on the server detail view, so only that operation's failure rolls it back.
type pendingResize struct {
	serverID string
	seq      uint64
}

func (m Model) doConfirmResize() (Model, tea.Cmd) { return m.doResizeStep(confirmResizeStep) }
func (m Model) doRevertResize() (Model, tea.Cmd)  { return m.doResizeStep(revertResizeStep) }

func (m Model) doResizeStep(step resizeStep) (Model, tea.Cmd) {
	if m.view == viewServerList && m.serverList.SelectionCount() > 0 {
		return m.doBulkResizeStep(step)
	}

	var id, name, status string
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			id, name, status = s.ID, s.Name, s.Status
		}
	case viewServerDetail:
		id, name, status = m.serverDetail.ServerID(), m.serverDetail.ServerName(), m.serverDetail.ServerStatus()
	}
	if id == "" {
		return m, nil
	}
	if status != "VERIFY_RESIZE" {
		m.statusBar.StickyHint = fmt.Sprintf("%s is not awaiting resize confirmation (status %s)", name, status)
		return m, nil
	}
	locks := []actionLock{m.lock("server", id, name)}
	if busy := m.busy(locks); busy != "" {
		return m.rejectBusy(busy)
	}
	client := m.client.Compute
	cmd := func() tea.Msg {
		ctx, cancel := actionCtx()
		defer cancel()
		if err := step.call(ctx, client, id); err != nil {
			m.logAudit(step.audit, "server", id, name, "error", err.Error())
			return shared.ServerActionErrMsg{Action: step.label, Name: name, Err: err}
		}
		m.logAudit(step.audit, "server", id, name, "success", "")
		return shared.ServerActionMsg{Action: step.label, Name: name}
	}
	m, tracked, seq := m.trackAction(locks, cmd)
	if m.view == viewServerDetail {
		m.serverDetail.SetPendingAction(step.pending)
		m.actions.pendingResize = pendingResize{serverID: id, seq: seq}
	}
	m.statusBar.StickyHint = fmt.Sprintf("%s resize of %s...", step.verb, name)
	return m, tracked
}

func (m Model) doBulkResizeStep(step resizeStep) (Model, tea.Cmd) {
	var eligible []modal.ServerRef
	var skipped []bulkItem
	var locks []actionLock
	for _, s := range m.serverList.SelectedServers() {
		ref := modal.ServerRef{ID: s.ID, Name: s.Name}
		if s.Status != "VERIFY_RESIZE" {
			skipped = append(skipped, bulkItem{ref: ref, err: fmt.Errorf("status %s, not VERIFY_RESIZE", s.Status)})
			continue
		}
		eligible = append(eligible, ref)
		locks = append(locks, m.lock("server", s.ID, s.Name))
	}
	if len(eligible) == 0 {
		m.statusBar.StickyHint = "No selected server is awaiting resize confirmation (VERIFY_RESIZE)"
		return m, nil
	}
	if busy := m.busy(locks); busy != "" {
		return m.rejectBusy(busy)
	}
	m.serverList.ClearSelection()
	client := m.client.Compute
	cmd := func() tea.Msg {
		res := bulkResultMsg{resource: "server", label: step.label, noun: "servers"}
		for _, it := range skipped {
			m.logAudit(step.audit, "server", it.ref.ID, it.ref.Name, "skipped", it.err.Error())
			res.skip(it.ref, it.err)
		}
		for _, ref := range eligible {
			ctx, cancel := actionCtx()
			err := step.call(ctx, client, ref.ID)
			cancel()
			if err != nil {
				m.logAudit(step.audit, "server", ref.ID, ref.Name, "error", err.Error())
				res.fail(ref, err)
				continue
			}
			m.logAudit(step.audit, "server", ref.ID, ref.Name, "success", "")
			res.ok(ref)
		}
		return res
	}
	m, tracked, _ := m.trackAction(locks, cmd)
	m.statusBar.StickyHint = fmt.Sprintf("%s resize of %d servers...", step.verb, len(eligible))
	return m, tracked
}

// rollbackPendingResize undoes the optimistic resize label when the
// operation that set it failed. Results of any other operation (for
// example a late failure for a server the user already left) are ignored.
func (m *Model) rollbackPendingResize(seq uint64, result tea.Msg) {
	if m.actions == nil || seq == 0 || m.actions.pendingResize.seq != seq {
		return
	}
	id := m.actions.pendingResize.serverID
	m.actions.pendingResize = pendingResize{}
	if _, failed := result.(shared.ServerActionErrMsg); !failed {
		return
	}
	if m.view == viewServerDetail && m.serverDetail.ServerID() == id {
		m.serverDetail.SetPendingAction("")
		m.statusBar.Hint = m.serverDetail.Hints()
	}
}
