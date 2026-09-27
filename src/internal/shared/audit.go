package shared

import (
	"sync/atomic"

	"github.com/larkly/lazystack/internal/audit"
)

// AuditRecord describes one mutation for the audit log.
type AuditRecord struct {
	Action       audit.ActionType
	ResourceType string
	ResourceID   string
	ResourceName string
	Details      map[string]string
	Err          error // nil when the mutation succeeded
}

// Audit is embedded in the result message of a UI command that mutates a
// cloud resource. The root model writes the record as the result leaves
// the command (off the Update loop, attributed to the cloud and project
// that issued it), so the mutation is recorded even if the result is later
// dropped because the user switched connections meanwhile. UI packages
// thus need no audit logger. The zero value records nothing.
type Audit struct {
	records []AuditRecord
	taken   *atomic.Bool
}

// NewAudit returns an Audit for a mutation of the named resource; err is
// the mutation's outcome.
func NewAudit(action audit.ActionType, resourceType, id, name string, err error) Audit {
	return Audits(AuditRecord{Action: action, ResourceType: resourceType, ResourceID: id, ResourceName: name, Err: err})
}

// Audits returns an Audit for several mutations reported by one result
// (for example one record per server of a bulk operation).
func Audits(records ...AuditRecord) Audit {
	if len(records) == 0 {
		return Audit{}
	}
	return Audit{records: records, taken: new(atomic.Bool)}
}

// WithDetails returns a with extra structured context attached to its
// records.
func (a Audit) WithDetails(details map[string]string) Audit {
	recs := make([]AuditRecord, len(a.records))
	for i, r := range a.records {
		r.Details = details
		recs[i] = r
	}
	a.records = recs
	return a
}

// TakeAudit returns the records the first time it is called on a (or any
// copy of it), so a result that passes several command wrappers is still
// recorded once.
func (a Audit) TakeAudit() ([]AuditRecord, bool) {
	if len(a.records) == 0 || !a.taken.CompareAndSwap(false, true) {
		return nil, false
	}
	return a.records, true
}

// Auditable is implemented by messages that embed Audit.
type Auditable interface {
	TakeAudit() ([]AuditRecord, bool)
}
