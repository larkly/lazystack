package lbview

import (
	"testing"

	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/testutil"
)

func TestLBViewTruncatesUnicodeSafely(t *testing.T) {
	for _, width := range []int{30, 50, 79, 100, 140} {
		m := New(nil, 0)
		m.SetSize(width, 40)
		m, _ = m.Update(lbsLoadedMsg{lbs: []loadbalancer.LoadBalancer{
			{ID: "l", Name: testutil.UnicodeName, VipAddress: "2001:db8:1234:5678:9abc:def0:1234:5678", ProvisioningStatus: "ACTIVE", OperatingStatus: "ONLINE"},
			{ID: "abcdefg", ProvisioningStatus: "ACTIVE"},
			{ID: "", ProvisioningStatus: "ERROR"},
		}})
		m, _ = m.Update(detailLoadedMsg{lbID: m.SelectedLB().ID,
			listeners: []loadbalancer.Listener{
				{ID: "li", Name: testutil.UnicodeName, Protocol: "HTTP", ProtocolPort: 80, DefaultPoolID: "p"},
				{ID: "lj", Protocol: "TCP", ProtocolPort: 443, DefaultPoolID: "abcdefghijk"},
			},
			pools: []loadbalancer.Pool{{ID: "p", Name: testutil.UnicodeName, Protocol: "HTTP", LBMethod: testutil.UnicodeName}},
		})
		// The narrow LB layout has fixed-width selector columns, so only the
		// wide layout is held to the width budget; every width must still
		// render valid UTF-8 and complete escape sequences.
		budget := 0
		if width >= narrowThreshold {
			budget = width
		}
		testutil.AssertRenderSafe(t, m.View(), budget)
	}
}
