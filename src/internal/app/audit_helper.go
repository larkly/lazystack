package app

import (
	"time"

	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/shared"
)

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
	if err := m.auditLogger.Log(entry); err != nil {
		// Called from command goroutines: never touch the model here. The
		// failure is handed to Update with the action's result instead.
		shared.Debugf("[audit] write failed: %v", err)
		if m.actions != nil {
			m.actions.recordAuditErr(err)
		}
	}
}
