package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/ui/dnslist"
	"github.com/larkly/lazystack/internal/ui/floatingiplist"
	"github.com/larkly/lazystack/internal/ui/keypairlist"
	"github.com/larkly/lazystack/internal/ui/networkview"
	"github.com/larkly/lazystack/internal/ui/routerview"
	"github.com/larkly/lazystack/internal/ui/secgroupview"
	"github.com/larkly/lazystack/internal/ui/volumelist"
)

func keyText(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)})
}

// listFilterCase opens a list view's "/" filter in the root model and reads
// back the filter text, so the same key-routing checks run on every view.
type listFilterCase struct {
	name  string
	open  func(m *Model) // put the view on screen with its filter active
	value func(m Model) string
}

func listFilterCases() []listFilterCase {
	return []listFilterCase{
		{
			name: "volumes",
			open: func(m *Model) {
				m.view = viewVolumeList
				m.volumeList = volumelist.New(nil, nil, 5*time.Second)
				m.volumeList, _ = m.volumeList.Update(keyText('/'))
			},
			value: func(m Model) string { return m.volumeList.FilterValue() },
		},
		{
			name: "floating IPs",
			open: func(m *Model) {
				m.view = viewFloatingIPList
				m.floatingIPList = floatingiplist.New(nil, 5*time.Second)
				m.floatingIPList, _ = m.floatingIPList.Update(keyText('/'))
			},
			value: func(m Model) string { return m.floatingIPList.FilterValue() },
		},
		{
			name: "key pairs",
			open: func(m *Model) {
				m.view = viewKeypairList
				m.keypairList = keypairlist.New(nil, 5*time.Second)
				m.keypairList, _ = m.keypairList.Update(keyText('/'))
			},
			value: func(m Model) string { return m.keypairList.FilterValue() },
		},
		{
			name: "security groups",
			open: func(m *Model) {
				m.view = viewSecGroupView
				m.secGroupView = secgroupview.New(nil, 5*time.Second)
				m.secGroupView, _ = m.secGroupView.Update(keyText('/'))
			},
			value: func(m Model) string { return m.secGroupView.FilterValue() },
		},
		{
			name: "networks",
			open: func(m *Model) {
				m.view = viewNetworkList
				m.networkView = networkview.New(nil, 5*time.Second)
				m.networkView, _ = m.networkView.Update(keyText('/'))
			},
			value: func(m Model) string { return m.networkView.FilterValue() },
		},
		{
			name: "routers",
			open: func(m *Model) {
				m.view = viewRouterView
				m.routerView = routerview.New(nil, 5*time.Second)
				m.routerView, _ = m.routerView.Update(keyText('/'))
			},
			value: func(m Model) string { return m.routerView.FilterValue() },
		},
		{
			name: "DNS zones",
			open: func(m *Model) {
				m.view = viewDNSList
				m.dnsList = dnslist.New(nil)
				m.dnsList, _ = m.dnsList.Update(keyText('/'))
			},
			value: func(m Model) string { return m.dnsList.FilterValue() },
		},
	}
}

// Typing into a list filter must never reach the global shortcuts: "q"
// would quit the app and digits would switch tabs.
func TestTypingQIntoListFilterDoesNotQuit(t *testing.T) {
	for _, c := range listFilterCases() {
		t.Run(c.name, func(t *testing.T) {
			m := newTestModel("dev", false)
			c.open(&m)
			if !m.textInputFocused() {
				t.Fatalf("filter is active but textInputFocused() = false")
			}
			view := m.view
			res, cmd := m.Update(keyText('q'))
			updated := res.(Model)
			if cmd != nil {
				if _, quit := cmd().(tea.QuitMsg); quit {
					t.Fatal("typing q into the filter quit the app")
				}
			}
			res, _ = updated.Update(keyText('2'))
			updated = res.(Model)
			if updated.view != view {
				t.Fatalf("typing 2 into the filter switched view to %v", updated.view)
			}
			if got := c.value(updated); got != "q2" {
				t.Fatalf("filter value = %q, want %q", got, "q2")
			}
		})
	}
}
