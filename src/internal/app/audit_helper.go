package app

import (
	"time"

	"github.com/larkly/lazystack/internal/audit"
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
	_ = m.auditLogger.Log(entry)
}
