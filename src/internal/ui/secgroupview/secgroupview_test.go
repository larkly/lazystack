package secgroupview

import (
	"testing"

	"github.com/larkly/lazystack/internal/network"
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
