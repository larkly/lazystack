package networkview

import (
	"testing"

	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/testutil"
)

func TestNetworkViewTruncatesUnicodeSafely(t *testing.T) {
	for _, width := range []int{30, 50, 79, 100, 140} {
		m := New(nil, 0)
		m.SetSize(width, 40)
		m, _ = m.Update(networksLoadedMsg{
			networks: []network.Network{{ID: "n", Name: testutil.UnicodeName, Status: "ACTIVE", SubnetIDs: []string{"s1", "s"}}},
			allSubnets: map[string]network.Subnet{
				"s1": {ID: "s1", Name: testutil.UnicodeName, CIDR: "2001:db8:1234:5678::/64", GatewayIP: "2001:db8:1234:5678::1", IPVersion: 6},
				"s":  {ID: "s", CIDR: "10.0.0.0/24", IPVersion: 4},
			},
		})
		m, _ = m.Update(detailLoadedMsg{netID: "n",
			ports: []network.Port{
				{ID: "p", Name: testutil.UnicodeName, DeviceID: "d", DeviceOwner: "compute:" + testutil.UnicodeName, Status: "ACTIVE",
					FixedIPs:       []network.FixedIP{{SubnetID: "s1", IPAddress: "2001:db8:1234:5678:9abc:def0:1234:5678"}},
					SecurityGroups: []string{"sg", "0123456789"}},
				{ID: "q", DeviceID: "abcdefghij", DeviceOwner: "network:router_interface"},
			},
			serverNames: map[string]string{"d": testutil.UnicodeName},
			sgNames:     map[string]string{},
		})
		testutil.AssertRenderSafe(t, m.View(), width)
	}
}
