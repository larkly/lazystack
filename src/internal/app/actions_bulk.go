package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/modal"
	"github.com/larkly/lazystack/internal/ui/vmpassword"
	"github.com/larkly/lazystack/internal/volume"
)

// bulkItem is one target of a bulk operation that did not succeed, with
// the failure (or skip) reason.
type bulkItem struct {
	ref modal.ServerRef
	err error
}

// bulkResultMsg is the structured outcome of a multi-target mutation:
// every target ends up in exactly one of succeeded, failed or skipped.
type bulkResultMsg struct {
	resource  string // "server", "volume", "image", "lb_member"
	label     string // action label, e.g. "stop" or "Delete volumes"
	noun      string // plural target noun, e.g. "servers"
	succeeded []modal.ServerRef
	failed    []bulkItem
	skipped   []bulkItem
}

func (r *bulkResultMsg) ok(ref modal.ServerRef) { r.succeeded = append(r.succeeded, ref) }
func (r *bulkResultMsg) fail(ref modal.ServerRef, err error) {
	r.failed = append(r.failed, bulkItem{ref: ref, err: err})
}
func (r *bulkResultMsg) skip(ref modal.ServerRef, err error) {
	r.skipped = append(r.skipped, bulkItem{ref: ref, err: err})
}

// summary describes the outcome with explicit counts.
func (r bulkResultMsg) summary() string {
	total := len(r.succeeded) + len(r.failed) + len(r.skipped)
	if len(r.failed) == 0 && len(r.skipped) == 0 {
		return fmt.Sprintf("%s %d %s", r.label, total, r.noun)
	}
	s := fmt.Sprintf("%s: %d of %d %s succeeded", r.label, len(r.succeeded), total, r.noun)
	if len(r.failed) > 0 {
		s += fmt.Sprintf(", %d failed", len(r.failed))
	}
	if len(r.skipped) > 0 {
		s += fmt.Sprintf(", %d skipped", len(r.skipped))
	}
	return s
}

func describeBulkItem(it bulkItem) string {
	name := it.ref.Name
	if name == "" || name == it.ref.ID {
		name = it.ref.ID
	} else {
		name = fmt.Sprintf("%s (%s)", name, it.ref.ID)
	}
	if it.ref.Action != "" {
		name += " [" + it.ref.Action + "]"
	}
	return fmt.Sprintf("%s: %v", name, it.err)
}

// handleBulkResult reports counts, lists every failed target, keeps the
// failed targets selected so they can be retried, and refreshes the
// affected view whatever the outcome.
func (m Model) handleBulkResult(r bulkResultMsg) (Model, tea.Cmd) {
	m.statusBar.Error = ""
	summary := r.summary()
	if len(r.failed) == 0 {
		m.statusBar.StickyHint = "✓ " + summary
	} else {
		m.statusBar.StickyHint = "⚠ " + summary
		var lines []string
		for _, it := range r.failed {
			lines = append(lines, describeBulkItem(it))
		}
		for _, it := range r.skipped {
			lines = append(lines, "skipped "+describeBulkItem(it))
		}
		m.errModal = modal.NewError(summary, errors.New(strings.Join(lines, "\n")))
		m.errModal.SetSize(m.width, m.height)
		m.activeModal = modalError
		ids := make([]string, len(r.failed))
		for i, it := range r.failed {
			ids[i] = it.ref.ID
		}
		switch r.resource {
		case "server":
			m.serverList.SelectIDs(ids)
		case "volume":
			m.volumeList.SelectIDs(ids)
		case "image":
			m.imageView.SelectIDs(ids)
		}
	}
	if r.resource == "server" {
		m.serverResize.Active = false
		return m, func() tea.Msg { return shared.RefreshServersMsg{} }
	}
	return m.forceRefreshActiveView()
}

func (m Model) executeBulkAction(client *gophercloud.ServiceClient, action modal.ConfirmAction) tea.Cmd {
	targets := action.Servers
	act := action.Action
	counts := countBulkActions(targets, act)
	return func() tea.Msg {
		label := act
		if len(counts) > 1 {
			label = fmt.Sprintf("mixed action (%s)", formatActionCounts(counts))
		}
		res := bulkResultMsg{resource: "server", label: label, noun: "servers"}
		var passwords []vmpassword.Credential
		for _, s := range targets {
			serverAction := s.Action
			if serverAction == "" {
				serverAction = act
			}
			ref := modal.ServerRef{ID: s.ID, Name: s.Name, Action: serverAction}
			var err error
			var auditAction audit.ActionType
			ctx, cancel := actionCtx()
			switch serverAction {
			case "delete":
				auditAction = audit.ActionDelete
				err = compute.DeleteServer(ctx, client, s.ID)
			case "soft reboot":
				auditAction = audit.ActionReboot
				err = compute.RebootServer(ctx, client, s.ID, servers.SoftReboot)
			case "hard reboot":
				auditAction = audit.ActionReboot
				err = compute.RebootServer(ctx, client, s.ID, servers.HardReboot)
			case "pause":
				auditAction = audit.ActionPause
				err = compute.PauseServer(ctx, client, s.ID)
			case "unpause":
				auditAction = audit.ActionUnpause
				err = compute.UnpauseServer(ctx, client, s.ID)
			case "suspend":
				auditAction = audit.ActionSuspend
				err = compute.SuspendServer(ctx, client, s.ID)
			case "resume":
				auditAction = audit.ActionResume
				err = compute.ResumeServer(ctx, client, s.ID)
			case "shelve":
				auditAction = audit.ActionShelve
				err = compute.ShelveServer(ctx, client, s.ID)
			case "unshelve":
				auditAction = audit.ActionUnshelve
				err = compute.UnshelveServer(ctx, client, s.ID)
			case "stop":
				auditAction = audit.ActionStop
				err = compute.StopServer(ctx, client, s.ID)
			case "start":
				auditAction = audit.ActionStart
				err = compute.StartServer(ctx, client, s.ID)
			case "lock":
				auditAction = audit.ActionLock
				err = compute.LockServer(ctx, client, s.ID)
			case "unlock":
				auditAction = audit.ActionUnlock
				err = compute.UnlockServer(ctx, client, s.ID)
			case "rescue":
				auditAction = audit.ActionRescue
				var adminPass string
				adminPass, err = compute.RescueServer(ctx, client, s.ID)
				if err == nil && adminPass != "" {
					passwords = append(passwords, vmpassword.Credential{Server: s.Name, Secret: adminPass})
				}
			case "unrescue":
				auditAction = audit.ActionUnrescue
				err = compute.UnrescueServer(ctx, client, s.ID)
			default:
				auditAction = audit.ActionUnknown
				err = fmt.Errorf("unsupported bulk action %q", serverAction)
			}
			cancel()
			if err != nil {
				m.logAudit(auditAction, "server", s.ID, s.Name, "error", err.Error())
				res.fail(ref, err)
			} else {
				m.logAudit(auditAction, "server", s.ID, s.Name, "success", "")
				res.ok(ref)
			}
		}
		if len(passwords) > 0 {
			return credentialsMsg{
				result: res,
				title:  "Rescue Passwords",
				note:   "Temporary rescue-mode passwords; they are not stored anywhere.",
				creds:  passwords,
			}
		}
		return res
	}
}

func (m Model) executeDeleteVolumesBulk(refs []modal.ServerRef) tea.Cmd {
	bsClient := m.client.BlockStorage
	if bsClient == nil {
		return nil
	}
	return func() tea.Msg {
		res := bulkResultMsg{resource: "volume", label: "Delete volumes", noun: "volumes"}
		for _, ref := range refs {
			shared.Debugf("[action] deleting volume %s", ref.Name)
			ctx, cancel := actionCtx()
			err := volume.DeleteVolume(ctx, bsClient, ref.ID)
			cancel()
			if err != nil {
				shared.Debugf("[action] delete volume %s failed: %s", ref.Name, err)
				m.logAudit(audit.ActionDelete, "volume", ref.ID, ref.Name, "error", err.Error())
				res.fail(ref, err)
				continue
			}
			m.logAudit(audit.ActionDelete, "volume", ref.ID, ref.Name, "success", "")
			res.ok(ref)
		}
		return res
	}
}

func (m Model) executeDetachVolumesBulk(refs []modal.ServerRef) tea.Cmd {
	computeC := m.client.Compute
	bsClient := m.client.BlockStorage
	if bsClient == nil || computeC == nil {
		return nil
	}
	return func() tea.Msg {
		res := bulkResultMsg{resource: "volume", label: "Detach volumes", noun: "volumes"}
		for _, ref := range refs {
			shared.Debugf("[action] detaching volume %s", ref.Name)
			ctx, cancel := actionCtx()
			vol, err := volume.GetVolume(ctx, bsClient, ref.ID)
			if err != nil {
				cancel()
				shared.Debugf("[action] detach volume %s failed: %s", ref.Name, err)
				m.logAudit(audit.ActionDetachVolume, "volume", ref.ID, ref.Name, "error", err.Error())
				res.fail(ref, err)
				continue
			}
			if !vol.IsAttached() {
				cancel()
				m.logAudit(audit.ActionDetachVolume, "volume", ref.ID, ref.Name, "skipped", "volume is not attached")
				res.skip(ref, errors.New("volume is not attached"))
				continue
			}
			var errs []string
			for _, att := range vol.Attachments {
				if err := volume.DetachVolume(ctx, computeC, att.ServerID, ref.ID); err != nil {
					shared.Debugf("[action] detach volume %s from server %s failed: %s", ref.Name, att.ServerID, err)
					errs = append(errs, fmt.Sprintf("from %s: %v", att.ServerID, err))
				}
			}
			cancel()
			if len(errs) > 0 {
				err := errors.New(strings.Join(errs, "; "))
				m.logAudit(audit.ActionDetachVolume, "volume", ref.ID, ref.Name, "error", err.Error())
				res.fail(ref, err)
				continue
			}
			m.logAudit(audit.ActionDetachVolume, "volume", ref.ID, ref.Name, "success", "")
			res.ok(ref)
		}
		return res
	}
}

func (m Model) executeDeleteImagesBulk(refs []modal.ServerRef) tea.Cmd {
	imgClient := m.client.Image
	if imgClient == nil {
		return nil
	}
	return func() tea.Msg {
		res := bulkResultMsg{resource: "image", label: "Delete images", noun: "images"}
		for _, ref := range refs {
			shared.Debugf("[action] deleting image %s", ref.Name)
			ctx, cancel := actionCtx()
			err := image.DeleteImage(ctx, imgClient, ref.ID)
			cancel()
			if err != nil {
				shared.Debugf("[action] delete image %s failed: %s", ref.Name, err)
				m.logAudit(audit.ActionDeleteImage, "image", ref.ID, ref.Name, "error", err.Error())
				res.fail(ref, err)
				continue
			}
			m.logAudit(audit.ActionDeleteImage, "image", ref.ID, ref.Name, "success", "")
			res.ok(ref)
		}
		return res
	}
}

// encodeLBMembersTarget packs the pool and load balancer captured when the
// bulk member confirmation opened.
func encodeLBMembersTarget(poolID, lbID string) string { return poolID + "|" + lbID }

func decodeLBMembersTarget(s string) (poolID, lbID string, ok bool) {
	parts := strings.SplitN(s, "|", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// executeDeleteLBMembersBulk deletes exactly the members captured in the
// confirmation, waiting for the load balancer to return to ACTIVE between
// deletions. If waiting fails, the remaining members are reported as not
// attempted, so every target is accounted for.
func (m Model) executeDeleteLBMembersBulk(action modal.ConfirmAction) tea.Cmd {
	poolID, lbID, ok := decodeLBMembersTarget(action.ServerID)
	if !ok || len(action.Servers) == 0 {
		return nil
	}
	lbClient := m.client.LoadBalancer
	members := action.Servers
	return func() tea.Msg {
		ctx, cancel := actionCtxLong()
		defer cancel()
		res := bulkResultMsg{resource: "lb_member", label: "Delete members", noun: "members"}
		for i, mem := range members {
			if i > 0 && lbID != "" {
				if err := loadbalancer.WaitForActive(ctx, lbClient, lbID, 60*time.Second); err != nil {
					shared.Debugf("[action] bulk delete wait failed: %s", err)
					err = fmt.Errorf("not attempted: waiting for load balancer failed: %w", err)
					for _, rest := range members[i:] {
						m.logAudit(audit.ActionDeleteLB, "lb_member", rest.ID, rest.Name, "error", err.Error())
						res.fail(rest, err)
					}
					break
				}
			}
			shared.Debugf("[action] bulk deleting member %s from pool %s (%d/%d)", mem.ID, poolID, i+1, len(members))
			if err := loadbalancer.DeleteMember(ctx, lbClient, poolID, mem.ID); err != nil {
				shared.Debugf("[action] bulk delete member %s failed: %s", mem.ID, err)
				m.logAudit(audit.ActionDeleteLB, "lb_member", mem.ID, mem.Name, "error", err.Error())
				res.fail(mem, err)
				continue
			}
			m.logAudit(audit.ActionDeleteLB, "lb_member", mem.ID, mem.Name, "success", "")
			res.ok(mem)
		}
		return res
	}
}
