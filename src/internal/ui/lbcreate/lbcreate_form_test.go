package lbcreate

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

func press(m Model, k tea.Key) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg(k))
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = press(m, tea.Key{Code: r, Text: string(r)})
	}
	return m
}

var ctrlS = tea.Key{Code: 's', Mod: tea.ModCtrl}

func twoSubnets() []network.Subnet {
	return []network.Subnet{
		{ID: "s-1", Name: "private", CIDR: "10.0.0.0/24"},
		{ID: "s-2", Name: "public", CIDR: "192.0.2.0/24"},
	}
}

func TestNewStartsOnNameFieldAndLoadsSubnets(t *testing.T) {
	m := New(nil, nil)
	if !m.Active || m.editMode || !m.subnetsLoading {
		t.Errorf("Active=%v editMode=%v subnetsLoading=%v, want active create form loading subnets", m.Active, m.editMode, m.subnetsLoading)
	}
	if m.currentField() != fieldName || !m.nameInput.Focused() {
		t.Errorf("field=%d focused=%v, want name focused", m.currentField(), m.nameInput.Focused())
	}
	if m.Init() == nil {
		t.Error("create form Init should fetch subnets")
	}
}

func TestTabCyclesThroughAllCreateFields(t *testing.T) {
	m := New(nil, nil)
	want := []int{fieldDesc, fieldSubnet, fieldSubmit, fieldCancel, fieldName}
	for i, w := range want {
		m, _ = press(m, tea.Key{Code: tea.KeyTab})
		if m.currentField() != w {
			t.Fatalf("tab %d: field=%d, want %d", i+1, m.currentField(), w)
		}
	}
	if !m.nameInput.Focused() {
		t.Error("name input should regain focus after wrapping")
	}
	m, _ = press(m, tea.Key{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.currentField() != fieldCancel {
		t.Errorf("shift+tab from name: field=%d, want fieldCancel", m.currentField())
	}
}

func TestEditModeHasFourFields(t *testing.T) {
	m := NewEdit(nil, "lb-1", "edge", "front door")
	if !m.editMode || m.lbID != "lb-1" || m.nameInput.Value() != "edge" || m.descInput.Value() != "front door" {
		t.Errorf("edit model wrong: edit=%v id=%q name=%q desc=%q", m.editMode, m.lbID, m.nameInput.Value(), m.descInput.Value())
	}
	if m.Init() != nil {
		t.Error("edit form must not fetch subnets")
	}
	want := []int{fieldDesc, fieldSubmit, fieldCancel, fieldName}
	for i, w := range want {
		m, _ = press(m, tea.Key{Code: tea.KeyTab})
		if m.currentField() != w {
			t.Fatalf("tab %d: field=%d, want %d", i+1, m.currentField(), w)
		}
	}
	if v := m.View(); !strings.Contains(v, "Edit Load Balancer") || strings.Contains(v, "Subnet") {
		t.Error("edit view should be titled Edit and omit the subnet row")
	}
}

func TestSubmitWithEmptyNameShowsErrorAndNoCommand(t *testing.T) {
	m := New(nil, nil)
	m, _ = m.Update(subnetsLoadedMsg{subnets: twoSubnets()})
	m, cmd := press(m, ctrlS)
	if cmd != nil || m.submitting {
		t.Errorf("cmd=%v submitting=%v, want neither", cmd != nil, m.submitting)
	}
	if m.err != "Name is required" || !strings.Contains(m.View(), "Name is required") {
		t.Errorf("err=%q, want it set and rendered", m.err)
	}
}

func TestEditSubmitWithEmptyNameIsRejected(t *testing.T) {
	m := NewEdit(nil, "lb-1", "", "")
	m, cmd := press(m, ctrlS)
	if cmd != nil || m.err != "Name is required" {
		t.Errorf("cmd=%v err=%q, want validation error without command", cmd != nil, m.err)
	}
}

func TestSubmitWithNameAndSubnetsStartsRequest(t *testing.T) {
	m := typeText(New(nil, nil), "edge")
	m, _ = m.Update(subnetsLoadedMsg{subnets: twoSubnets()})
	m, cmd := press(m, ctrlS)
	if cmd == nil || !m.submitting || m.err != "" {
		t.Errorf("cmd=%v submitting=%v err=%q, want request started", cmd != nil, m.submitting, m.err)
	}
	m, c := press(m, tea.Key{Code: tea.KeyEscape})
	if !m.Active || c != nil {
		t.Error("esc must be ignored while submitting")
	}
}

func TestEditSubmitStartsRequestWithoutSubnets(t *testing.T) {
	m := NewEdit(nil, "lb-1", "edge", "")
	m, cmd := press(m, ctrlS)
	if cmd == nil || !m.submitting {
		t.Errorf("cmd=%v submitting=%v, want update started", cmd != nil, m.submitting)
	}
}

func TestEscCancelsFromTextAndButtonFields(t *testing.T) {
	for _, f := range []int{fieldName, fieldDesc, fieldSubnet, fieldSubmit, fieldCancel} {
		m := New(nil, nil)
		m.focusField = f
		m.updateFocus()
		m, _ = press(m, tea.Key{Code: tea.KeyEscape})
		if m.Active {
			t.Errorf("esc on field %d should close the form", f)
		}
	}
}

func TestSubnetPickerSelectsAndAdvances(t *testing.T) {
	m := New(nil, nil)
	m, _ = m.Update(subnetsLoadedMsg{subnets: twoSubnets()})
	m.focusField = fieldSubnet
	m.updateFocus()
	m, _ = press(m, tea.Key{Code: tea.KeyEnter})
	if !m.pickerOpen {
		t.Fatal("enter on the subnet field should open the picker")
	}
	if !strings.HasPrefix(m.Hints(), "↑↓ navigate") {
		t.Errorf("picker hints = %q", m.Hints())
	}
	m, _ = press(m, tea.Key{Code: tea.KeyDown})
	m, _ = press(m, tea.Key{Code: tea.KeyDown})
	if m.pickerCursor != 1 {
		t.Errorf("pickerCursor=%d, want clamped to 1", m.pickerCursor)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyEnter})
	if m.pickerOpen || m.selectedSubnet != 1 || m.currentField() != fieldSubmit {
		t.Errorf("open=%v selected=%d field=%d, want picker closed on subnet 1 with submit focused", m.pickerOpen, m.selectedSubnet, m.currentField())
	}
	if !strings.Contains(m.View(), "public (192.0.2.0/24)") {
		t.Error("view should show the chosen subnet")
	}
}

func TestPickerEscClosesPickerNotForm(t *testing.T) {
	m := New(nil, nil)
	m, _ = m.Update(subnetsLoadedMsg{subnets: twoSubnets()})
	m.pickerOpen = true
	m, _ = press(m, tea.Key{Code: tea.KeyEscape})
	if m.pickerOpen || !m.Active {
		t.Errorf("pickerOpen=%v Active=%v, want only the picker closed", m.pickerOpen, m.Active)
	}
}

func TestSubnetFetchErrorShowsErrorAndStopsLoading(t *testing.T) {
	m := New(nil, nil)
	m, _ = m.Update(subnetsFetchErrMsg{err: errors.New("boom")})
	if m.subnetsLoading || m.err == "" || !m.Active || !strings.Contains(m.View(), m.err) {
		t.Errorf("loading=%v err=%q Active=%v, want error rendered", m.subnetsLoading, m.err, m.Active)
	}
}

func TestCreateErrorKeepsFormOpen(t *testing.T) {
	m := typeText(New(nil, nil), "edge")
	m, _ = m.Update(lbCreateErrMsg{err: errors.New("boom")})
	if !m.Active || m.submitting || m.err == "" {
		t.Errorf("Active=%v submitting=%v err=%q, want open form with error", m.Active, m.submitting, m.err)
	}
}

func TestSuccessReportsActionPerMode(t *testing.T) {
	m, cmd := typeText(New(nil, nil), "edge").Update(lbCreatedMsg{})
	got, ok := cmd().(shared.ResourceActionMsg)
	if m.Active || !ok || got.Action != "Created LB" || got.Name != "edge" {
		t.Errorf("create: Active=%v result=%#v, want closed with Created LB / edge", m.Active, got)
	}

	m, cmd = NewEdit(nil, "lb-1", "edge", "").Update(lbCreatedMsg{})
	got, ok = cmd().(shared.ResourceActionMsg)
	if m.Active || !ok || got.Action != "Updated LB" {
		t.Errorf("edit: Active=%v result=%#v, want closed with Updated LB", m.Active, got)
	}
}

func TestPickerAndErrorRenderAtNarrowWidth(t *testing.T) {
	m := New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	m, _ = m.Update(subnetsLoadedMsg{subnets: twoSubnets()})
	m.err = "Name is required"
	testutil.AssertRenderSafe(t, m.View(), 40)
	m.pickerOpen = true
	testutil.AssertRenderSafe(t, m.View(), 40)
}
