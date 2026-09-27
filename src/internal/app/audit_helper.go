package app

import (
	"encoding/json"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/shared"
)

// auditSink is the part of the model an audit write needs: the logger, the
// identity to attribute entries to, and where to report write failures.
// Commands capture it when they are issued.
type auditSink struct {
	logger  *audit.Logger
	cloud   string
	project string
	actions *actionState
}

func (m Model) auditSink() auditSink {
	return auditSink{logger: m.auditLogger, cloud: m.cloudName, project: m.currentProjectID, actions: m.actions}
}

// logAudit records an action to the audit logger.
func (m Model) logAudit(action audit.ActionType, resourceType, resourceID, resourceName, result, errMsg string) {
	m.logAuditDetails(action, resourceType, resourceID, resourceName, result, errMsg, nil)
}

// logAuditDetails is logAudit with extra structured context (for example
// the subnet and port of a router interface).
func (m Model) logAuditDetails(action audit.ActionType, resourceType, resourceID, resourceName, result, errMsg string, details map[string]string) {
	m.auditSink().log(action, resourceType, resourceID, resourceName, result, errMsg, details)
}

func (s auditSink) log(action audit.ActionType, resourceType, resourceID, resourceName, result, errMsg string, details map[string]string) {
	if s.logger == nil || !s.logger.IsEnabled() {
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
		Cloud:        s.cloud,
		Project:      s.project,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Result:       result,
		Error:        errMsg,
		Details:      raw,
	}
	if err := s.logger.Log(entry); err != nil {
		// Called from command goroutines: never touch the model here. The
		// failure is handed to Update with the action's result instead.
		shared.Debugf("[audit] write failed: %v", err)
		if s.actions != nil {
			s.actions.recordAuditErr(err)
		}
	}
}

// auditResults wraps cmd so that a result carrying a shared.Audit record
// (the outcome of a mutation run by a UI view, picker or form) is written
// to the audit log as it leaves the command: in the command goroutine, and
// attributed to the cloud and project current when the command was issued.
// Mutations run by the root model itself log directly and carry no record,
// so nothing is recorded twice.
func (m Model) auditResults(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return m.auditSink().wrap(cmd)
}

func (s auditSink) wrap(cmd tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			wrapped := make(tea.BatchMsg, len(batch))
			for i, c := range batch {
				if c != nil {
					wrapped[i] = s.wrap(c)
				}
			}
			return wrapped
		}
		recs, ok := takeAudit(msg)
		if !ok {
			return msg
		}
		for _, rec := range recs {
			result, errText := "success", ""
			if rec.Err != nil {
				result, errText = "error", rec.Err.Error()
			}
			s.log(rec.Action, rec.ResourceType, rec.ResourceID, rec.ResourceName, result, errText, rec.Details)
		}
		if s.actions != nil {
			if err := s.actions.takeAuditErr(); err != nil {
				return tea.BatchMsg{
					func() tea.Msg { return msg },
					func() tea.Msg { return actionResultMsg{auditErr: err} },
				}
			}
		}
		return msg
	}
}

// takeAudit extracts the audit record of a command result, looking through
// the root model's own result envelopes.
func takeAudit(msg tea.Msg) ([]shared.AuditRecord, bool) {
	switch m := msg.(type) {
	case actionResultMsg:
		return takeAudit(m.msg)
	case connScopedMsg:
		return takeAudit(m.msg)
	case shared.Auditable:
		return m.TakeAudit()
	}
	return nil, false
}
