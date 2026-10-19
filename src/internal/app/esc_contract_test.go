package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/routerview"
)

var (
	escPress   = tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	enterPress = tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
)

func escStep(t *testing.T, m Model, k tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()
	res, cmd := m.Update(k)
	return res.(Model), cmd
}

// Every list view follows one esc contract: clear the committed filter,
// then the selection, and only then let esc mean back-navigation. A list
// reached by a jump from server detail must not jump back while it still
// has a filter to clear.
func TestEscClearsListFilterBeforeBackNavigation(t *testing.T) {
	for _, c := range listFilterCases() {
		t.Run(c.name, func(t *testing.T) {
			m := newTestModel("dev", false)
			m.returnToView = viewServerDetail
			m.serverDetail = testServerDetailWithServer("srv-1", "srv-1")
			c.open(&m)
			view := m.view
			m, _ = escStep(t, m, keyText('x'))
			m, _ = escStep(t, m, enterPress)
			if m.textInputFocused() || c.value(m) != "x" {
				t.Fatalf("setup: filter focused=%v value=%q, want committed x", m.textInputFocused(), c.value(m))
			}

			m, _ = escStep(t, m, escPress)
			if m.view != view || c.value(m) != "" {
				t.Fatalf("first esc: view=%v filter=%q; want the filter cleared on the same view", m.view, c.value(m))
			}

			m, _ = escStep(t, m, escPress)
			if m.view != viewServerDetail {
				t.Fatalf("second esc: view=%v; with nothing left to clear esc goes back to server detail", m.view)
			}
		})
	}
}

// DNS is a tab like any other: esc with nothing to clear must not leave it.
func TestEscOnDNSListStaysOnTheTab(t *testing.T) {
	m := newTestModel("dev", false)
	for _, c := range listFilterCases() {
		if c.name == "DNS zones" {
			c.open(&m)
		}
	}
	m, _ = escStep(t, m, keyText('x'))
	m, _ = escStep(t, m, enterPress)
	m, _ = escStep(t, m, escPress) // clears the filter
	m, cmd := escStep(t, m, escPress)
	if m.view != viewDNSList {
		t.Fatalf("view = %v after esc, want DNS list", m.view)
	}
	if cmd != nil {
		if vc, ok := cmd().(shared.ViewChangeMsg); ok {
			t.Fatalf("esc on DNS asked for view %q", vc.View)
		}
	}
}

// On the router view esc from a detail pane first returns focus to the
// router list; only then does it mean back-navigation.
func TestEscFromRouterDetailReturnsToListFirst(t *testing.T) {
	m := newTestModel("dev", false)
	m.returnToView = viewServerDetail
	m.serverDetail = testServerDetailWithServer("srv-1", "srv-1")
	m.view = viewRouterView
	m.routerView = routerview.New(nil, 5*time.Second)
	m, _ = escStep(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if m.routerView.FocusedPane() == routerview.FocusSelector {
		t.Fatal("setup: tab did not leave the router list")
	}
	m, _ = escStep(t, m, escPress)
	if m.view != viewRouterView || m.routerView.FocusedPane() != routerview.FocusSelector {
		t.Fatalf("first esc: view=%v pane=%v, want the router list", m.view, m.routerView.FocusedPane())
	}
	m, _ = escStep(t, m, escPress)
	if m.view != viewServerDetail {
		t.Fatalf("second esc: view=%v, want back to server detail", m.view)
	}
}
