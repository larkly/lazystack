package lbcreate

import (
	"testing"

	"github.com/larkly/lazystack/internal/network"
)

// Submitting before the subnet list arrives says it is still loading;
// "No subnets available" is only claimed once the list is in and empty.
func TestSubmitDistinguishesLoadingFromNoSubnets(t *testing.T) {
	m := New(nil, nil)
	m.nameInput.SetValue("edge")
	m, cmd := m.submit()
	if cmd != nil || m.err != "Loading subnets…" {
		t.Fatalf("while loading: cmd=%v err=%q, want %q", cmd != nil, m.err, "Loading subnets…")
	}

	m, _ = m.Update(subnetsLoadedMsg{subnets: []network.Subnet{}})
	m, cmd = m.submit()
	if cmd != nil || m.err != "No subnets available" {
		t.Fatalf("after load: cmd=%v err=%q, want %q", cmd != nil, m.err, "No subnets available")
	}
}
