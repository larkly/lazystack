package secgroupview

import (
	"testing"

	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/testutil"
)

func TestSecGroupViewTruncatesUnicodeSafely(t *testing.T) {
	for _, width := range []int{30, 50, 79, 100, 140} {
		m := New(nil, 0)
		m.SetSize(width, 40)
		m, _ = m.Update(sgLoadedMsg{groups: []network.SecurityGroup{{
			ID: "g", Name: testutil.UnicodeName, Description: testutil.UnicodeName,
			Rules: []network.SecurityRule{
				{ID: "r1", Direction: "ingress", EtherType: "IPv6", Protocol: "ipv6-icmp", RemoteIPPrefix: "2001:db8:1234:5678:9abc::/80"},
				{ID: "r2", Direction: "ingress", EtherType: "IPv4", Protocol: "tcp", PortRangeMin: 1024, PortRangeMax: 65535, RemoteGroupID: "x"},
				{ID: "r3", Direction: "egress", EtherType: "IPv4", RemoteGroupID: testutil.UnicodeName},
			},
		}, {ID: "h", Name: "短"}}})
		m, _ = m.Update(detailLoadedMsg{sgID: "g",
			servers:     []serverRef{{ID: "s", Name: testutil.UnicodeName, Status: "ACTIVE"}},
			ports:       []network.Port{{ID: "p", DeviceID: "s", FixedIPs: []network.FixedIP{{IPAddress: "2001:db8:1234:5678:9abc:def0:1234:5678"}}}},
			serverNames: map[string]string{"s": testutil.UnicodeName},
		})
		testutil.AssertRenderSafe(t, m.View(), width)
	}
}
