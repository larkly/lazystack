package app

import (
	"slices"
	"testing"

	"github.com/larkly/lazystack/internal/shared"
)

// A volume opened from server detail's volumes pane must return there after
// a successful action. The volume list is not the active tab in that case,
// so it would never receive its refresh reply and would stay loading.
func TestVolumeActionFromServerDetailReturnsToServerDetail(t *testing.T) {
	m, requests := listFixture(t)
	m.activeTab = 0 // Servers
	m.tabInited[0] = true
	m.view = viewServerDetail
	m.returnToView = viewSecGroupView

	res, _ := m.Update(shared.NavigateToDetailMsg{Resource: "volume", ID: "vol"})
	m = res.(Model)
	if m.view != viewVolumeDetail {
		t.Fatalf("view=%v want volume detail", m.view)
	}

	res, cmd := m.Update(shared.ResourceActionMsg{Action: "Detached", Name: "fixture-volume"})
	got := res.(Model)
	if got.view != viewServerDetail {
		t.Fatalf("view=%v want server detail", got.view)
	}
	if got.statusBar.CurrentView != "serverdetail" {
		t.Errorf("status view=%q want serverdetail", got.statusBar.CurrentView)
	}
	if !got.nav.IsEmpty() {
		t.Errorf("nav stack not balanced: %d entries left", got.nav.Len())
	}
	if got.returnToView != viewSecGroupView {
		t.Errorf("returnToView=%v, server detail lost its origin", got.returnToView)
	}
	if got.serverDetail.Server() != nil {
		t.Fatal("fixture server detail should start unloaded")
	}

	// The refresh replies must reach the view the user landed on.
	before := len(*requests)
	for _, msg := range commandMessages(cmd) {
		res, _ = got.Update(msg)
		got = res.(Model)
	}
	if !slices.Contains((*requests)[before:], "/servers/srv") {
		t.Fatalf("refresh did not fetch the server: %v", (*requests)[before:])
	}
	if s := got.serverDetail.Server(); s == nil || s.Name != "fixture-server" {
		t.Fatalf("server detail did not receive its refresh reply: %+v", s)
	}
}

// Opened from the volume list, a successful action still returns to the
// volume list, and its refresh reply reaches it because Volumes is the
// active tab.
func TestVolumeActionFromVolumeListReturnsToVolumeList(t *testing.T) {
	m, requests := listFixture(t)
	m.activeTab = 1 // Volumes
	m.tabInited[1] = true
	m.view = viewVolumeList

	res, _ := m.Update(shared.NavigateToDetailMsg{Resource: "volume", ID: "vol"})
	m = res.(Model)
	res, cmd := m.Update(shared.ResourceActionMsg{Action: "Deleted", Name: "fixture-volume"})
	got := res.(Model)
	if got.view != viewVolumeList {
		t.Fatalf("view=%v want volume list", got.view)
	}
	if !got.nav.IsEmpty() {
		t.Errorf("nav stack not balanced: %d entries left", got.nav.Len())
	}
	before := len(*requests)
	for _, msg := range commandMessages(cmd) {
		res, _ = got.Update(msg)
		got = res.(Model)
	}
	if !slices.Contains((*requests)[before:], "/volumes/detail") {
		t.Fatalf("refresh did not fetch volumes: %v", (*requests)[before:])
	}
}
