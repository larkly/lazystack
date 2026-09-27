package usermanagement

import (
	"errors"
	"testing"

	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/compute"
)

func TestUserMutationAuditRecords(t *testing.T) {
	enabled := compute.User{ID: "u1", Name: "alice", Enabled: true}
	disabled := compute.User{ID: "u2", Name: "bob", Enabled: false}
	boom := errors.New("forbidden")
	cases := []struct {
		name string
		p    pendingAction
		err  error
		want audit.ActionType
		id   string
	}{
		{"disable", pendingAction{actionToggle, enabled}, nil, audit.ActionDisableUser, "u1"},
		{"enable", pendingAction{actionToggle, disabled}, nil, audit.ActionEnableUser, "u2"},
		{"delete", pendingAction{actionDelete, enabled}, nil, audit.ActionDelete, "u1"},
		{"failed delete", pendingAction{actionDelete, disabled}, boom, audit.ActionDelete, "u2"},
	}
	for _, tc := range cases {
		recs, ok := userAudit(tc.p, tc.err).TakeAudit()
		if !ok || len(recs) != 1 {
			t.Fatalf("%s: records = %+v", tc.name, recs)
		}
		r := recs[0]
		if r.Action != tc.want || r.ResourceType != "user" || r.ResourceID != tc.id || r.Err != tc.err {
			t.Errorf("%s: record = %+v", tc.name, r)
		}
	}
}
