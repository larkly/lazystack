package app

import (
	"encoding/json"
	"time"

	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/shared"
)

// logAudit records an action to the audit logger.
func (m Model) logAudit(action audit.ActionType, resourceType, resourceID, resourceName, result, errMsg string) {
	m.logAuditDetails(action, resourceType, resourceID, resourceName, result, errMsg, nil)
}

// logAuditDetails is logAudit with extra structured context (for example
// the subnet and port of a router interface).
func (m Model) logAuditDetails(action audit.ActionType, resourceType, resourceID, resourceName, result, errMsg string, details map[string]string) {
	if m.auditLogger == nil || !m.auditLogger.IsEnabled() {
		return
	}
	var raw json.RawMessage
	if len(details) > 0 {
		if b, err := json.Marshal(details); err == nil {
			raw = b
		}
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
		Details:      raw,
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
