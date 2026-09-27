package app

import (
	"fmt"
	"strings"

	"charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/ui/modal"
)

// actionState is session-wide bookkeeping for mutations started from the
// root model. inflight and seq are only touched from Update.
type actionState struct {
	inflight map[string]string // lock key -> display name of the resource
	seq      uint64
}

func newActionState() *actionState {
	return &actionState{inflight: make(map[string]string)}
}

// ensureActions returns the model's action state, creating it for models
// built without New (tests).
func (m *Model) ensureActions() *actionState {
	if m.actions == nil {
		m.actions = newActionState()
	}
	return m.actions
}

// actionLock names one resource a pending mutation holds.
type actionLock struct {
	key, name string
}

// actionResultMsg carries the result of a tracked mutation back to Update
// so it can release the resource locks that mutation held and correlate
// optimistic UI state with the operation (seq) that set it.
type actionResultMsg struct {
	seq   uint64
	locks []string
	msg   tea.Msg
}

// lock scopes a resource lock to the current connection (the client is
// replaced on every connect/project switch) and the immutable resource ID.
func (m Model) lock(kind, id, name string) actionLock {
	if name == "" {
		name = id
	}
	return actionLock{key: fmt.Sprintf("%p|%s|%s", m.client, kind, id), name: name}
}

// confirmLocks lists the resources a confirmed action mutates.
func (m Model) confirmLocks(a modal.ConfirmAction) []actionLock {
	refs := func(kind string) []actionLock {
		out := make([]actionLock, 0, len(a.Servers))
		for _, s := range a.Servers {
			out = append(out, m.lock(kind, s.ID, s.Name))
		}
		return out
	}
	one := func(kind string) []actionLock { return []actionLock{m.lock(kind, a.ServerID, a.Name)} }
	switch a.Action {
	case "delete_volumes_bulk", "detach_volumes_bulk":
		return refs("volume")
	case "delete_images_bulk":
		return refs("image")
	case "delete_lb_members_bulk":
		return refs("lb_member")
	}
	if len(a.Servers) > 0 {
		return refs("server")
	}
	switch a.Action {
	case "delete":
		locks := one("server")
		if a.DeleteVolumes {
			for _, vid := range a.VolumeIDs {
				locks = append(locks, m.lock("volume", vid, vid))
			}
		}
		return locks
	case "soft reboot", "hard reboot", "pause", "unpause", "suspend", "resume",
		"shelve", "unshelve", "stop", "start", "lock", "unlock", "rescue", "unrescue":
		return one("server")
	case "delete_volume", "detach_volume":
		return one("volume")
	case "release_fip", "disassociate_fip":
		return one("floating_ip")
	case "delete_router":
		return one("router")
	case "remove_router_interface":
		if t, ok := decodeRouterInterfaceTarget(a.ServerID); ok {
			return []actionLock{m.lock("router", t.routerID, a.Name)}
		}
	case "delete_port":
		return one("port")
	case "delete_network":
		return one("network")
	case "delete_subnet":
		return one("subnet")
	case "delete_sg":
		return one("security_group")
	case "delete_sg_rule":
		return one("security_group_rule")
	case "delete_lb":
		return one("load_balancer")
	case "delete_lb_listener":
		return one("lb_listener")
	case "delete_lb_pool":
		return one("lb_pool")
	case "delete_lb_monitor":
		return one("lb_monitor")
	case "delete_lb_member":
		if parts := strings.SplitN(a.ServerID, "|", 2); len(parts) == 2 {
			return []actionLock{m.lock("lb_member", parts[1], a.Name)}
		}
	case "delete_keypair":
		return one("keypair")
	case "delete_image", "deactivate_image", "reactivate_image":
		return one("image")
	}
	return nil
}

// busy returns a display name for the first lock already held, or "".
func (m Model) busy(locks []actionLock) string {
	if m.actions == nil {
		return ""
	}
	for _, l := range locks {
		if _, held := m.actions.inflight[l.key]; held {
			return l.name
		}
	}
	return ""
}

// rejectBusy reports that a resource already has a pending mutation.
func (m Model) rejectBusy(name string) (Model, tea.Cmd) {
	m.statusBar.StickyHint = fmt.Sprintf("%s already has an operation in progress; wait for it to finish", name)
	return m, nil
}

// trackAction takes the given locks for the lifetime of cmd and wraps its
// result so Update releases them on every outcome (success, error or
// timeout: every action command is bounded by a context deadline).
// It returns the operation's sequence number.
func (m Model) trackAction(locks []actionLock, cmd tea.Cmd) (Model, tea.Cmd, uint64) {
	if cmd == nil {
		return m, nil, 0
	}
	st := m.ensureActions()
	st.seq++
	seq := st.seq
	keys := make([]string, 0, len(locks))
	for _, l := range locks {
		st.inflight[l.key] = l.name
		keys = append(keys, l.key)
	}
	return m, func() tea.Msg {
		return actionResultMsg{seq: seq, locks: keys, msg: cmd()}
	}, seq
}

// runConfirmedAction executes a confirmed dialog action unless one of the
// resources it touches already has a mutation in flight.
func (m Model) runConfirmedAction(a modal.ConfirmAction) (Model, tea.Cmd) {
	locks := m.confirmLocks(a)
	if name := m.busy(locks); name != "" {
		return m.rejectBusy(name)
	}
	next, cmd := m.executeAction(a)
	next, cmd, _ = next.trackAction(locks, cmd)
	return next, cmd
}

// handleActionResult releases the finished operation's locks and then
// processes the wrapped result like any other message.
func (m Model) handleActionResult(msg actionResultMsg) (Model, tea.Cmd) {
	if m.actions != nil {
		for _, k := range msg.locks {
			delete(m.actions.inflight, k)
		}
	}
	if msg.msg == nil {
		return m, nil
	}
	next, cmd := m.Update(msg.msg)
	return next.(Model), cmd
}
