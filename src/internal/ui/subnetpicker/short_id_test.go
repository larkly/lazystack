package subnetpicker

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/testutil"
)

func TestUnnamedShortIDSubnetsDoNotPanic(t *testing.T) {
	for _, id := range []string{"", "s", "abcdefg", "abcdefgh", "0f8e2c5a-1b3d-4e6f-8a9b-0c1d2e3f4a5b", "子網子網子網子網子網"} {
		m := New(nil, "router", "edge")
		m.SetSize(80, 25)
		m, _ = m.Update(subnetsLoadedMsg{[]network.Subnet{{ID: id, CIDR: "10.0.0.0/24", GatewayIP: "10.0.0.1"}}})
		testutil.AssertRenderSafe(t, m.View(), 0)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		testutil.AssertRenderSafe(t, m.View(), 0)
	}
}
