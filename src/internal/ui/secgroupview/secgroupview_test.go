package secgroupview

import (
	"errors"
	"testing"

	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
)

func rulesFixture(ids ...string) []network.SecurityRule {
	rules := make([]network.SecurityRule, len(ids))
	for i, id := range ids {
		rules[i] = network.SecurityRule{ID: id, Direction: "ingress", Protocol: "tcp"}
	}
	return rules
}

// A same-ID refresh that removes rules must not leave the rule cursor past
// the end of the list (which would make SelectedRuleID empty while the Rules
// pane is focused).
func TestSameGroupRefreshClampsRuleCursor(t *testing.T) {
	m := New(nil, 0)
	m, _ = m.Update(sgLoadedMsg{groups: []network.SecurityGroup{{ID: "sg", Name: "web", Rules: rulesFixture("r1", "r2", "r3")}}})
	m.focus = FocusRules
	m.ruleCursor = 2
	if got := m.SelectedRuleID(); got == "" {
		t.Fatal("fixture should select the third rule")
	}

	m, _ = m.Update(sgLoadedMsg{groups: []network.SecurityGroup{{ID: "sg", Name: "web", Rules: rulesFixture("r1")}}})
	if m.ruleCursor != 0 || m.SelectedRuleID() != "r1" {
		t.Fatalf("ruleCursor=%d selected=%q, want clamped to the remaining rule", m.ruleCursor, m.SelectedRuleID())
	}

	m, _ = m.Update(sgLoadedMsg{groups: []network.SecurityGroup{{ID: "sg", Name: "web"}}})
	if m.ruleCursor != 0 || m.SelectedRuleID() != "" || m.SelectedRule() != nil {
		t.Fatalf("empty rules: ruleCursor=%d selected=%q", m.ruleCursor, m.SelectedRuleID())
	}
}

// Background ticks must not stack group or detail fetches, and slow older
// responses must never overwrite the results of newer fetches.
func TestTickRefreshesDoNotOverlapOrRegress(t *testing.T) {
	m := New(nil, 0)
	m, _ = m.Update(sgLoadedMsg{groups: []network.SecurityGroup{{ID: "sg", Name: "web"}}})
	m, _ = m.Update(detailLoadedMsg{seq: m.detailRefresh.Seq(), sgID: "sg"})

	m, first := m.Update(shared.TickMsg{})
	if first == nil {
		t.Fatal("tick did not fetch")
	}
	staleList, staleDetail := m.refresh.Seq(), m.detailRefresh.Seq()
	if _, again := m.Update(shared.TickMsg{}); again != nil {
		t.Fatal("tick started a second fetch while one was in flight")
	}

	m.ForceRefresh()
	m, _ = m.Update(sgLoadedMsg{seq: m.refresh.Seq(), groups: []network.SecurityGroup{{ID: "sg", Name: "new"}}})
	m, _ = m.Update(detailLoadedMsg{seq: m.detailRefresh.Seq(), sgID: "sg", ports: []network.Port{{ID: "p-new"}}})
	m, _ = m.Update(sgLoadedMsg{seq: staleList, groups: []network.SecurityGroup{{ID: "sg", Name: "old"}}})
	m, _ = m.Update(sgErrMsg{seq: staleList, err: errors.New("stale")})
	m, _ = m.Update(detailLoadedMsg{seq: staleDetail, sgID: "sg", ports: []network.Port{{ID: "p-old"}}})
	m, _ = m.Update(detailErrMsg{seq: staleDetail, sgID: "sg", err: errors.New("stale")})
	if m.groups[0].Name != "new" || m.err != "" {
		t.Fatalf("stale group list applied: %v err=%q", m.groups, m.err)
	}
	if len(m.ports) != 1 || m.ports[0].ID != "p-new" || m.detailErr != "" {
		t.Fatalf("stale detail applied: %v err=%q", m.ports, m.detailErr)
	}
	if _, next := m.Update(shared.TickMsg{}); next == nil {
		t.Fatal("tick blocked after the newest fetches completed")
	}
}
