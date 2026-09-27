package app

import (
	"fmt"
	"net/http"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/ui/secgroupview"
)

func secGroupFixture(t *testing.T, groupName string) Model {
	t.Helper()
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/security-groups":
			fmt.Fprintf(w, `{"security_groups":[{"id":"sg","name":%q,"security_group_rules":[]}]}`, groupName)
		case "/ports":
			fmt.Fprint(w, `{"ports":[]}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	m.secGroupView = secgroupview.New(m.client.Network, m.refreshInterval)
	m.secGroupView.SetComputeClient(m.client.Compute)
	loadViewData(m.secGroupView.Init(), func(msg tea.Msg) tea.Cmd {
		var cmd tea.Cmd
		m.secGroupView, cmd = m.secGroupView.Update(msg)
		return cmd
	})
	if m.secGroupView.SelectedGroupID() != "sg" {
		t.Fatalf("fixture did not load group: %q", m.secGroupView.SelectedGroupID())
	}
	m.view = viewSecGroupView
	return m
}

func focusSecGroupPane(t *testing.T, m Model, pane any) Model {
	t.Helper()
	for range 5 {
		if m.secGroupView.FocusedPane() == pane {
			return m
		}
		m.secGroupView, _ = m.secGroupView.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	}
	t.Fatalf("could not focus pane %v", pane)
	return m
}

func TestSecGroupDeleteOnEmptyRulesPaneDoesNotTargetGroup(t *testing.T) {
	m := secGroupFixture(t, "web")
	m = focusSecGroupPane(t, m, secgroupview.FocusRules)
	if m.secGroupView.SelectedRuleID() != "" {
		t.Fatal("fixture should have no selectable rule")
	}
	res, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Mod: tea.ModCtrl}))
	got := res.(Model)
	if got.activeModal != modalNone {
		t.Fatalf("ctrl+d on empty rules pane opened %q confirmation", got.confirm.Action)
	}
}

func TestSecGroupDeleteOnSelectorKeepsGroupDeleteAndDefaultProtection(t *testing.T) {
	m := secGroupFixture(t, "web")
	m = focusSecGroupPane(t, m, secgroupview.FocusSelector)
	res, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Mod: tea.ModCtrl}))
	got := res.(Model)
	if got.activeModal != modalConfirm || got.confirm.Action != "delete_sg" || got.confirm.ServerID != "sg" {
		t.Fatalf("selector ctrl+d: modal=%v action=%q", got.activeModal, got.confirm.Action)
	}

	m = secGroupFixture(t, "default")
	m = focusSecGroupPane(t, m, secgroupview.FocusSelector)
	res, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Mod: tea.ModCtrl}))
	if res.(Model).activeModal != modalNone {
		t.Fatal("default group must not be deletable")
	}
}
