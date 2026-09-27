package routerview

import (
	"testing"

	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/testutil"
)

func TestRouterViewTruncatesUnicodeSafely(t *testing.T) {
	for _, width := range []int{30, 50, 79, 100, 140} {
		m := New(nil, 0)
		m.SetSize(width, 30)
		m, _ = m.Update(routersLoadedMsg{routers: []network.Router{
			{ID: "r", Name: testutil.UnicodeName, Status: "ACTIVE", AdminStateUp: true,
				ExternalGatewayIPv4: "203.0.113.10", ExternalGatewayIPv6: "2001:db8:ffff:ffff:ffff:ffff:ffff:1",
				Routes: []network.Route{{DestinationCIDR: "2001:db8:1234:5678:9abc::/80", NextHop: "2001:db8::fffe:1"}}},
			{ID: "abcdefg", Status: "ACTIVE"},
			{ID: "", Status: "DOWN"},
		}})
		m, _ = m.Update(namesLoadedMsg{
			networkNames: map[string]string{"n1": testutil.UnicodeName},
			subnetToNet:  map[string]string{"s1": "n1"},
		})
		m, _ = m.Update(detailLoadedMsg{seq: m.detailRefresh.Seq(), routerID: "r", interfaces: []network.RouterInterface{
			{SubnetID: "s1", PortID: "p1", IPAddress: "2001:db8:1234:5678:9abc:def0:1234:5678"},
			{SubnetID: "s", PortID: "p", IPAddress: "10.0.0.1"},
		}})
		testutil.AssertRenderSafe(t, m.View(), width)
	}
}
