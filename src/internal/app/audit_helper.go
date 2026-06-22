package app

import (
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/audit"
)

// actionTypeFromString maps UI action strings to audit.ActionType.
func actionTypeFromString(s string) audit.ActionType {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "delete"):
		return audit.ActionDelete
	case strings.Contains(s, "reboot"):
		return audit.ActionReboot
	case strings.Contains(s, "pause"):
		return audit.ActionPause
	case strings.Contains(s, "unpause"):
		return audit.ActionUnpause
	case strings.Contains(s, "suspend"):
		return audit.ActionSuspend
	case strings.Contains(s, "resume"):
		return audit.ActionResume
	case strings.Contains(s, "shelve"):
		return audit.ActionShelve
	case strings.Contains(s, "unshelve"):
		return audit.ActionUnshelve
	case strings.Contains(s, "stop"):
		return audit.ActionStop
	case strings.Contains(s, "start"):
		return audit.ActionStart
	case strings.Contains(s, "lock"):
		return audit.ActionLock
	case strings.Contains(s, "unlock"):
		return audit.ActionUnlock
	case strings.Contains(s, "rescue"):
		return audit.ActionRescue
	case strings.Contains(s, "unrescue"):
		return audit.ActionUnrescue
	case strings.Contains(s, "resize"):
		return audit.ActionResize
	case strings.Contains(s, "rebuild"):
		return audit.ActionRebuild
	case strings.Contains(s, "rename"):
		return audit.ActionRename
	case strings.Contains(s, "snapshot"):
		return audit.ActionSnapshot
	case strings.Contains(s, "migrate"):
		return audit.ActionMigrate
	case strings.Contains(s, "evacuate"):
		return audit.ActionEvacuate
	case strings.Contains(s, "force delete"):
		return audit.ActionForceDelete
	case strings.Contains(s, "reset state"):
		return audit.ActionResetState
	default:
		return audit.ActionUnknown
	}
}

// logAudit records an action to the audit logger.
func (m Model) logAudit(action audit.ActionType, resourceType, resourceID, resourceName, result, errMsg string) {
	if m.auditLogger == nil || !m.auditLogger.IsEnabled() {
		return
	}
	entry := audit.Entry{
		Timestamp:    time.Now().UTC(),
		Cloud:        m.cloudName,
		Project:      m.currentProjectID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Result:       result,
		Error:        errMsg,
	}
	_ = m.auditLogger.Log(entry)
}
