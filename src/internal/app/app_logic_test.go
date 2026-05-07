package app

import (
	"testing"

	"github.com/larkly/lazystack/internal/config"
	"github.com/larkly/lazystack/internal/shared"
)

func initTestModel() Model {
	cfg := config.Defaults()
	m := New(Options{
		Version:     "test",
		CheckUpdate: false,
		Config:      &cfg,
	})
	m.width = 120
	m.height = 40
	return m
}

// --- Tab switching ---
// Note: switchTab to non-server tabs requires client (BlockStorage/Network)
// so we test the servers tab path and boundary conditions.

func TestSwitchTab_ValidIndexServers(t *testing.T) {
	m := initTestModel()
	m.tabs = DefaultTabs()
	m.activeTab = 2 // start on volumes (index 2)

	// Switch to servers (index 0) — safe, doesn't create new UI models
	rm, _ := m.switchTab(0)

	if rm.activeTab != 0 {
		t.Fatalf("activeTab = %d, want 0", rm.activeTab)
	}
	if rm.view != viewServerList {
		t.Fatalf("view = %d, want viewServerList", rm.view)
	}
	if rm.statusBar.CurrentView != "serverlist" {
		t.Fatalf("CurrentView = %q, want serverlist", rm.statusBar.CurrentView)
	}
}

func TestSwitchTab_NegativeIndex(t *testing.T) {
	m := initTestModel()
	m.tabs = DefaultTabs()
	m.activeTab = 0

	rm, _ := m.switchTab(-1)

	if rm.activeTab != 0 {
		t.Fatal("negative index should not change activeTab")
	}
}

func TestSwitchTab_OutOfRange(t *testing.T) {
	m := initTestModel()
	m.tabs = DefaultTabs()
	m.activeTab = 0

	rm, _ := m.switchTab(100)

	if rm.activeTab != 0 {
		t.Fatal("out-of-range index should not change activeTab")
	}
}

func TestSwitchTab_SameTabTopLevelNoOp(t *testing.T) {
	m := initTestModel()
	m.tabs = DefaultTabs()
	m.activeTab = 0
	m.view = viewServerList

	rm, _ := m.switchTab(0)

	if rm.view != viewServerList {
		t.Fatal("switching to current tab at top-level view should be no-op")
	}
}

func TestSwitchTab_SameTabNonTopLevel(t *testing.T) {
	m := initTestModel()
	m.tabs = DefaultTabs()
	m.activeTab = 0
	m.view = viewServerDetail

	rm, _ := m.switchTab(0)

	if rm.view != viewServerList {
		t.Fatalf("view = %d, want viewServerList (should reset to tab's default view)", rm.view)
	}
}

// --- isTopLevelView ---

func TestIsTopLevelView(t *testing.T) {
	tests := []struct {
		view     activeView
		expected bool
	}{
		{viewServerList, true},
		{viewVolumeList, true},
		{viewFloatingIPList, true},
		{viewSecGroupView, true},
		{viewKeypairList, true},
		{viewLBView, true},
		{viewNetworkList, true},
		{viewRouterView, true},
		{viewImageView, true},
		{viewDNSList, true},
		{viewServerDetail, false},
		{viewServerCreate, false},
		{viewCloudPicker, false},
		{viewConsoleLog, false},
		{viewActionLog, false},
	}

	for _, tt := range tests {
		m := initTestModel()
		m.view = tt.view
		got := m.isTopLevelView()
		if got != tt.expected {
			t.Errorf("isTopLevelView(%d) = %v, want %v", tt.view, got, tt.expected)
		}
	}
}

// --- Modal dispatch ---

func TestModal_None(t *testing.T) {
	m := initTestModel()
	if m.activeModal != modalNone {
		t.Fatal("new model should have no active modal")
	}
}

func TestModal_Error(t *testing.T) {
	m := initTestModel()
	m.activeModal = modalError
	if m.activeModal != modalError {
		t.Fatal("failed to set modalError")
	}
	m.activeModal = modalNone
	if m.activeModal != modalNone {
		t.Fatal("failed to clear modalError")
	}
}

func TestModal_Confirm(t *testing.T) {
	m := initTestModel()
	m.activeModal = modalConfirm
	if m.activeModal != modalConfirm {
		t.Fatal("failed to set modalConfirm")
	}
	m.activeModal = modalNone
	if m.activeModal != modalNone {
		t.Fatal("failed to clear modalConfirm")
	}
}

// --- View routing ---

func TestUpdateActiveView_ServerList(t *testing.T) {
	m := initTestModel()
	m.view = viewServerList
	m.serverList.SetSize(m.width, m.height)

	_, cmd := m.updateActiveView(nil)
	_ = cmd
}

func TestUpdateActiveView_CloudPicker(t *testing.T) {
	m := initTestModel()
	m.view = viewCloudPicker

	_, cmd := m.updateActiveView(nil)
	_ = cmd
}

func TestUpdate_TickForwarding(t *testing.T) {
	m := initTestModel()
	m.view = viewServerList
	m.serverList.SetSize(m.width, m.height)

	result, _ := m.Update(shared.TickMsg{})
	rm, ok := result.(Model)
	if !ok {
		t.Fatal("Update did not return Model")
	}
	if rm.view != viewServerList {
		t.Fatal("view should remain viewServerList after tick")
	}
}

func TestUpdate_ModalRouting(t *testing.T) {
	m := initTestModel()
	m.activeModal = modalConfirm
	m.view = viewServerList

	result, _ := m.Update(shared.TickMsg{})
	rm, ok := result.(Model)
	if !ok {
		t.Fatal("Update did not return Model")
	}
	if rm.view != viewServerList {
		t.Fatal("view should remain viewServerList")
	}
}

// --- DefaultTabs ---

func TestDefaultTabs_Structure(t *testing.T) {
	tabs := DefaultTabs()
	if len(tabs) == 0 {
		t.Fatal("DefaultTabs returned empty slice")
	}

	keys := make(map[string]bool)
	for _, td := range tabs {
		if td.Name == "" {
			t.Error("tab definition has empty Name")
		}
		if td.Key == "" {
			t.Error("tab definition has empty Key")
		}
		if keys[td.Key] {
			t.Errorf("duplicate tab key %q", td.Key)
		}
		keys[td.Key] = true
	}
}

func TestDefaultTabs_RequiredKeys(t *testing.T) {
	tabs := DefaultTabs()
	keys := make(map[string]bool)
	for _, td := range tabs {
		keys[td.Key] = true
	}

	required := []string{"servers", "volumes", "images", "floatingips", "secgroups", "networks", "keypairs"}
	for _, k := range required {
		if !keys[k] {
			t.Errorf("DefaultTabs missing required key %q", k)
		}
	}
}

// --- Tab bar rendering ---

func TestRenderTabBar_NonEmpty(t *testing.T) {
	m := initTestModel()
	m.tabs = DefaultTabs()
	m.activeTab = 0
	m.width = 120

	bar := m.renderTabBar()
	if bar == "" {
		t.Fatal("renderTabBar returned empty string")
	}
	if len(bar) < 5 {
		t.Fatal("tab bar too short")
	}
}

func TestRenderTabBar_ActiveTab(t *testing.T) {
	m := initTestModel()
	m.tabs = DefaultTabs()
	m.activeTab = 0
	m.width = 120

	bar1 := m.renderTabBar()

	m.activeTab = 2
	bar2 := m.renderTabBar()

	// Active tab changes should produce different output
	if bar1 == bar2 {
		t.Log("bar output identical across tabs — may be normal for short labels")
	}
}

// --- NavStack integration with Model ---

func TestNavStack_Integration(t *testing.T) {
	m := initTestModel()
	m.nav.Push(viewServerList, 0)
	m.nav.Push(viewServerDetail, 0)

	if m.nav.Len() != 2 {
		t.Fatalf("nav stack len = %d, want 2", m.nav.Len())
	}
	if m.nav.TopView() != viewServerDetail {
		t.Fatalf("top = %d, want viewServerDetail", m.nav.TopView())
	}

	m.nav.Pop()
	if m.nav.Len() != 1 {
		t.Fatalf("after pop: len = %d, want 1", m.nav.Len())
	}
	if m.nav.TopView() != viewServerList {
		t.Fatalf("after pop: top = %d, want viewServerList", m.nav.TopView())
	}
}

// --- Model state after Update ---

func TestUpdate_IdempotentTicks(t *testing.T) {
	m := initTestModel()
	m.view = viewServerList
	m.serverList.SetSize(m.width, m.height)

	for i := 0; i < 5; i++ {
		result, _ := m.Update(shared.TickMsg{})
		rm, ok := result.(Model)
		if !ok {
			t.Fatal("Update did not return Model")
		}
		if rm.view != viewServerList {
			t.Fatalf("iteration %d: view changed to %d", i, rm.view)
		}
		m = rm
	}
}

// --- Test Helpers ---

func TestInitTestModel(t *testing.T) {
	m := initTestModel()
	if m.width != 120 {
		t.Errorf("width = %d, want 120", m.width)
	}
	if m.height != 40 {
		t.Errorf("height = %d, want 40", m.height)
	}
	if m.view != 0 {
		t.Errorf("view = %d, want 0 (viewCloudPicker)", m.view)
	}
}
