package fippicker

import (
	"testing"

	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

// Allocation, association and the cleanup release are audited as separate
// actions, so a release is never mistaken for a disassociation.
func TestAllocateAuditRecords(t *testing.T) {
	type rec struct {
		action audit.ActionType
		failed bool
	}
	cases := []struct {
		name string
		fake *fakeNeutron
		want []rec
	}{
		{"associated", &fakeNeutron{associateOK: true},
			[]rec{{audit.ActionAllocateFIP, false}, {audit.ActionAttachFIP, false}}},
		{"released after failed association", &fakeNeutron{releaseOK: true},
			[]rec{{audit.ActionAllocateFIP, false}, {audit.ActionAttachFIP, true}, {audit.ActionReleaseFIP, false}}},
		{"release failed too", &fakeNeutron{},
			[]rec{{audit.ActionAllocateFIP, false}, {audit.ActionAttachFIP, true}, {audit.ActionReleaseFIP, true}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, cleanup := testutil.FakeServiceClient(tc.fake)
			defer cleanup()
			msg := New(client, "server-1", "web").allocateAndAssociate("ext", testTarget)()
			a, ok := msg.(shared.Auditable)
			if !ok {
				t.Fatalf("%T carries no audit record", msg)
			}
			got, ok := a.TakeAudit()
			if !ok || len(got) != len(tc.want) {
				t.Fatalf("records = %+v, want %d", got, len(tc.want))
			}
			for i, w := range tc.want {
				g := got[i]
				if g.Action != w.action || (g.Err != nil) != w.failed || g.ResourceType != "floating_ip" || g.ResourceID != "new-fip" {
					t.Errorf("record %d = %+v, want %v failed=%v", i, g, w.action, w.failed)
				}
			}
			if got[0].Details["server_id"] != "server-1" {
				t.Errorf("allocation not tied to the server: %+v", got[0].Details)
			}
		})
	}
}
