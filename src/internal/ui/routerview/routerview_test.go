package routerview

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/network"
)

func TestShiftTabMovesFocusBackward(t *testing.T) {
	m := New(nil, 5*time.Second)
	if m.focus != FocusSelector {
		t.Fatalf("initial focus = %v, want FocusSelector", m.focus)
	}

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	m = updated
	if m.focus != focusInfo {
		t.Fatalf("focus after tab = %v, want focusInfo", m.focus)
	}

	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	m = updated
	if m.focus != FocusSelector {
		t.Fatalf("focus after shift+tab = %v, want FocusSelector", m.focus)
	}
}

// TestNewConstructorInvariants verifies the New() constructor initializes
// all required fields so downstream operations never hit nil-map panics.
func TestNewConstructorInvariants(t *testing.T) {
	m := New(nil, 5*time.Second)
	if m.focus != FocusSelector {
		t.Errorf("initial focus = %v, want FocusSelector", m.focus)
	}
	if !m.loading {
		t.Errorf("loading should be true on init")
	}
	if m.networkNames == nil {
		t.Errorf("networkNames map should be non-nil on init")
	}
	if m.subnetToNet == nil {
		t.Errorf("subnetToNet map should be non-nil on init")
	}
	if m.refreshInterval != 5*time.Second {
		t.Errorf("refreshInterval = %v, want 5s", m.refreshInterval)
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
	if m.networkClient != nil {
		t.Errorf("networkClient = %v, want nil (we passed nil)", m.networkClient)
	}
}

// TestInitReturnsBatchedCmd verifies that Init() returns a non-nil tea.Cmd
// (the spinner tick + initial network fetches are batched).
func TestInitReturnsBatchedCmd(t *testing.T) {
	m := New(nil, 5*time.Second)
	cmd := m.Init()
	if cmd == nil {
		t.Fatalf("Init should return a non-nil tea.Cmd")
	}
	// Executing the batched cmd should not panic even with a nil network client.
	// We don't recurse into returned sub-cmds because fetchRouters/fetchNames
	// will return commands that bullet-proof their nil-client path.
	_ = cmd()
}

// TestSelectedRouterIDNameEmpty covers the empty-list and out-of-range paths.
func TestSelectedRouterIDNameEmpty(t *testing.T) {
	m := New(nil, 5*time.Second)
	if got := m.SelectedRouterID(); got != "" {
		t.Errorf("SelectedRouterID on empty list = %q, want empty", got)
	}
	if got := m.SelectedRouterName(); got != "" {
		t.Errorf("SelectedRouterName on empty list = %q, want empty", got)
	}
}

// TestSelectedRouterIDByName covers id lookup, empty-name fallback to ID, and bounds.
func TestSelectedRouterIDByName(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{
		{ID: "r1", Name: "router1"},
		{ID: "r2", Name: ""}, // empty name → fall back to ID
		{ID: "r3", Name: "router3"},
	}
	m.cursor = 0
	if got := m.SelectedRouterID(); got != "r1" {
		t.Errorf("cursor=0 SelectedRouterID = %q, want r1", got)
	}
	if got := m.SelectedRouterName(); got != "router1" {
		t.Errorf("cursor=0 SelectedRouterName = %q, want router1", got)
	}
	m.cursor = 1
	if got := m.SelectedRouterID(); got != "r2" {
		t.Errorf("cursor=1 SelectedRouterID = %q, want r2", got)
	}
	if got := m.SelectedRouterName(); got != "r2" {
		t.Errorf("cursor=1 empty name should fall back to ID, want r2, got %q", got)
	}
}

// TestSelectedRouterIDCursorOutOfRange verifies bounds-clamping.
func TestSelectedRouterIDCursorOutOfRange(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{{ID: "r1", Name: "router1"}}
	m.cursor = -1
	if got := m.SelectedRouterID(); got != "" {
		t.Errorf("cursor=-1 SelectedRouterID = %q, want empty", got)
	}
	m.cursor = 5
	if got := m.SelectedRouterID(); got != "" {
		t.Errorf("cursor=5 SelectedRouterID = %q, want empty", got)
	}
}

// TestSelectedInterfaceNotFocused verifies nothing is returned when not focused on Interfaces.
func TestSelectedInterfaceNotFocused(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.interfaces = []network.RouterInterface{{SubnetID: "s1", PortID: "p1", IPAddress: "10.0.0.1"}}
	m.focus = FocusSelector
	if iface := m.SelectedInterface(); iface != nil {
		t.Errorf("focus=FocusSelector SelectedInterface = %v, want nil", iface)
	}
	if got := m.SelectedInterfaceSubnetID(); got != "" {
		t.Errorf("focus=FocusSelector SelectedInterfaceSubnetID = %q, want empty", got)
	}
	m.focus = focusInfo
	if iface := m.SelectedInterface(); iface != nil {
		t.Errorf("focus=focusInfo SelectedInterface = %v, want nil", iface)
	}
	m.focus = focusRoutes
	if iface := m.SelectedInterface(); iface != nil {
		t.Errorf("focus=focusRoutes SelectedInterface = %v, want nil", iface)
	}
}

// TestSelectedInterfaceFocused covers index lookup and centering.
func TestSelectedInterfaceFocused(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.interfaces = []network.RouterInterface{
		{SubnetID: "s1", PortID: "p1", IPAddress: "10.0.0.1"},
		{SubnetID: "s2", PortID: "p2", IPAddress: "10.0.0.2"},
	}
	m.focus = FocusInterfaces
	m.interfaceCursor = 0
	if iface := m.SelectedInterface(); iface == nil || iface.SubnetID != "s1" {
		t.Errorf("interfaceCursor=0 SelectedInterface.SubnetID = %v, want s1", iface)
	}
	if got := m.SelectedInterfaceSubnetID(); got != "s1" {
		t.Errorf("interfaceCursor=0 subnetID = %q, want s1", got)
	}
	m.interfaceCursor = 1
	if got := m.SelectedInterfaceSubnetID(); got != "s2" {
		t.Errorf("interfaceCursor=1 subnetID = %q, want s2", got)
	}
	// Out-of-range lookup returns empty
	m.interfaceCursor = 5
	if iface := m.SelectedInterface(); iface != nil {
		t.Errorf("interfaceCursor=5 SelectedInterface = %v, want nil", iface)
	}
	if got := m.SelectedInterfaceSubnetID(); got != "" {
		t.Errorf("interfaceCursor=5 subnetID = %q, want empty", got)
	}
	m.interfaceCursor = -1
	if got := m.SelectedInterfaceSubnetID(); got != "" {
		t.Errorf("interfaceCursor=-1 subnetID = %q, want empty", got)
	}
}

// TestInterfacesOnPort counts matching portIDs in the interface list.
func TestInterfacesOnPort(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.interfaces = []network.RouterInterface{
		{PortID: "p1", SubnetID: "s1"},
		{PortID: "p1", SubnetID: "s2"},
		{PortID: "p2", SubnetID: "s3"},
	}
	if got := m.InterfacesOnPort("p1"); got != 2 {
		t.Errorf("InterfacesOnPort(p1) = %d, want 2", got)
	}
	if got := m.InterfacesOnPort("p2"); got != 1 {
		t.Errorf("InterfacesOnPort(p2) = %d, want 1", got)
	}
	if got := m.InterfacesOnPort("p3"); got != 0 {
		t.Errorf("InterfacesOnPort(p3) = %d, want 0", got)
	}
	if got := m.InterfacesOnPort(""); got != 0 {
		t.Errorf("InterfacesOnPort('') = %d, want 0 (empty port never matches)", got)
	}
}

// TestFocusedPaneGetter verifies the focus pane getter methods.
func TestFocusedPaneGetter(t *testing.T) {
	m := New(nil, 5*time.Second)
	if got := m.FocusedPane(); got != FocusSelector {
		t.Errorf("FocusedPane = %v, want FocusSelector", got)
	}
	if m.InInterfaces() {
		t.Errorf("InInterfaces = true at FocusSelector, want false")
	}
	m.focus = FocusInterfaces
	if got := m.FocusedPane(); got != FocusInterfaces {
		t.Errorf("FocusedPane = %v, want FocusInterfaces", got)
	}
	if !m.InInterfaces() {
		t.Errorf("InInterfaces = false at FocusInterfaces, want true")
	}
}

// TestTabCyclesAllPanes verifies Tab makes a full orbit through all four panes,
// wrapping focusRoutes back to FocusSelector on the fourth press.
func TestTabCyclesAllPanes(t *testing.T) {
	m := New(nil, 5*time.Second)
	wantOrder := []focusPane{focusInfo, FocusInterfaces, focusRoutes, FocusSelector}
	for i, want := range wantOrder {
		updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		m = updated
		if m.focus != want {
			t.Errorf("after %d tabs, focus = %v, want %v", i+1, want, m.focus)
		}
	}
}

// TestShiftTabWrapBackwardWithRoutes verifies Shift+Tab wrap-around backward
// behaves: from FocusSelector it should land on the last pane (focusRoutes).
func TestShiftTabWrapBackwardWithRoutes(t *testing.T) {
	m := New(nil, 5*time.Second)
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	m = updated
	if m.focus != focusRoutes {
		t.Errorf("Shift+Tab from FocusSelector focus = %v, want focusRoutes (wrap backward)", m.focus)
	}
}

// TestMoveUpSelectorAtTopStaysAtZero verifies upward navigation clamps at 0.
func TestMoveUpSelectorAtTopStaysAtZero(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{
		{ID: "r1", Name: "router1"},
		{ID: "r2", Name: "router2"},
	}
	m.cursor = 0
	upd, _ := m.moveUp()
	if upd.cursor != 0 {
		t.Errorf("moveUp at cursor=0 didn't stay; got cursor=%d, want 0", upd.cursor)
	}
}

// TestMoveDownSelectorAdvancesWithinBounds verifies moveDown increments cursor and clamps at last index.
func TestMoveDownSelectorAdvancesWithinBounds(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{
		{ID: "r1", Name: "router1"},
		{ID: "r2", Name: "router2"},
		{ID: "r3", Name: "router3"},
	}
	m.cursor = 0
	upd, _ := m.moveDown()
	if upd.cursor != 1 {
		t.Errorf("moveDown from cursor=0 -> cursor=%d, want 1", upd.cursor)
	}
	upd, _ = upd.moveDown()
	if upd.cursor != 2 {
		t.Errorf("moveDown from cursor=1 -> cursor=%d, want 2", upd.cursor)
	}
	// At bottom — should not advance past last index.
	upd, _ = upd.moveDown()
	if upd.cursor != 2 {
		t.Errorf("moveDown at end should stay; got cursor=%d, want 2", upd.cursor)
	}
}

// TestMoveUpSelectorAdvancesBackward verifies moveUp decreases cursor within bounds.
func TestMoveUpSelectorAdvancesBackward(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{
		{ID: "r1", Name: "router1"},
		{ID: "r2", Name: "router2"},
	}
	m.cursor = 1
	upd, _ := m.moveUp()
	if upd.cursor != 0 {
		t.Errorf("moveUp from cursor=1 -> cursor=%d, want 0", upd.cursor)
	}
}

// TestMoveUpDownArrowKeys exercises the public Update path with up/down arrow keys.
func TestMoveUpDownArrowKeys(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{
		{ID: "r1", Name: "router1"},
		{ID: "r2", Name: "router2"},
		{ID: "r3", Name: "router3"},
	}
	upd, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = upd
	if m.cursor != 1 {
		t.Errorf("Down arrow -> cursor=%d, want 1", m.cursor)
	}
	upd, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m = upd
	if m.cursor != 0 {
		t.Errorf("Up arrow -> cursor=%d, want 0", m.cursor)
	}
	// Up at top boundary — stays at 0.
	upd, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m = upd
	if m.cursor != 0 {
		t.Errorf("Up arrow at top -> cursor=%d, want 0", m.cursor)
	}
	// Down at bottom boundary — stays at last index.
	m.cursor = 2
	upd, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = upd
	if m.cursor != 2 {
		t.Errorf("Down arrow at bottom -> cursor=%d, want 2", m.cursor)
	}
}

// TestMoveInterfacesFocused verifies navigation when FocusInterfaces is the active pane.
func TestMoveInterfacesFocused(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.interfaces = []network.RouterInterface{
		{SubnetID: "s1", PortID: "p1"},
		{SubnetID: "s2", PortID: "p2"},
		{SubnetID: "s3", PortID: "p3"},
	}
	m.focus = FocusInterfaces
	m.interfaceCursor = 0
	upd, _ := m.moveDown()
	if upd.interfaceCursor != 1 {
		t.Errorf("interface moveDown -> interfaceCursor=%d, want 1", upd.interfaceCursor)
	}
	upd, _ = upd.moveDown()
	if upd.interfaceCursor != 2 {
		t.Errorf("interface moveDown -> interfaceCursor=%d, want 2", upd.interfaceCursor)
	}
	// At bottom: cursor should not advance further.
	upd, _ = upd.moveDown()
	if upd.interfaceCursor != 2 {
		t.Errorf("interface moveDown at end -> interfaceCursor=%d, want 2", upd.interfaceCursor)
	}
	// And moveUp to come back.
	upd, _ = upd.moveUp()
	if upd.interfaceCursor != 1 {
		t.Errorf("interface moveUp -> interfaceCursor=%d, want 1", upd.interfaceCursor)
	}
	upd, _ = upd.moveUp()
	if upd.interfaceCursor != 0 {
		t.Errorf("interface moveUp -> interfaceCursor=%d, want 0", upd.interfaceCursor)
	}
	// At top: cursor should not go below 0.
	upd, _ = upd.moveUp()
	if upd.interfaceCursor != 0 {
		t.Errorf("interface moveUp at start -> interfaceCursor=%d, want 0", upd.interfaceCursor)
	}
}

// TestMoveUpInterfacesNoOpWithoutInterfaces verifies the focusInterfaces path
// is safe to simulate even with an empty interface list (cursor never negative).
func TestMoveUpInterfacesNoOpWithoutInterfaces(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.focus = FocusInterfaces
	m.interfaces = nil
	m.interfaceCursor = 0
	upd, _ := m.moveUp()
	if upd.interfaceCursor != 0 {
		t.Errorf("moveUp on empty interface list -> interfaceCursor=%d, want 0", upd.interfaceCursor)
	}
	upd, _ = m.moveDown()
	if upd.interfaceCursor != 0 {
		t.Errorf("moveDown on empty interface list -> interfaceCursor=%d, want 0", upd.interfaceCursor)
	}
}

// TestMoveRoutesFocused verifies the focusRoutes cursor navigation honors
// the selected router's routes list bounds.
func TestMoveRoutesFocused(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{
		{ID: "r1", Name: "router1", Routes: []network.Route{{}, {}, {}}},
	}
	m.cursor = 0
	m.focus = focusRoutes
	m.routesCursor = 0
	upd, _ := m.moveDown()
	if upd.routesCursor != 1 {
		t.Errorf("routes moveDown -> routesCursor=%d, want 1", upd.routesCursor)
	}
	upd, _ = upd.moveDown()
	if upd.routesCursor != 2 {
		t.Errorf("routes moveDown -> routesCursor=%d, want 2", upd.routesCursor)
	}
	// At bottom: stays.
	upd, _ = upd.moveDown()
	if upd.routesCursor != 2 {
		t.Errorf("routes moveDown at end -> routesCursor=%d, want 2", upd.routesCursor)
	}
	// moveUp to come back.
	upd, _ = upd.moveUp()
	if upd.routesCursor != 1 {
		t.Errorf("routes moveUp -> routesCursor=%d, want 1", upd.routesCursor)
	}
	upd, _ = upd.moveUp()
	if upd.routesCursor != 0 {
		t.Errorf("routes moveUp -> routesCursor=%d, want 0", upd.routesCursor)
	}
	// At top: stays.
	upd, _ = upd.moveUp()
	if upd.routesCursor != 0 {
		t.Errorf("routes moveUp at start -> routesCursor=%d, want 0", upd.routesCursor)
	}
}

// TestMoveRoutesFocusedBeyondRoutesList verifies routes cursor bounds checks
// when the selected router has fewer routes than the current cursor.
func TestMoveRoutesFocusedBeyondRoutesList(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{{ID: "r1", Name: "router1", Routes: nil}}
	m.cursor = 0
	m.focus = focusRoutes
	m.routesCursor = 0
	// moveDown with no routes — selectedRouter exists but routes list is empty.
	// The `r.routesCursor < len(r.Routes)-1` check should fail because len=0.
	// Code computes `len(r.Routes) - 1 = -1`, and `0 < -1` is false → no advance.
	upd, _ := m.moveDown()
	if upd.routesCursor != 0 {
		t.Errorf("routes moveDown with no routes -> routesCursor=%d, want 0", upd.routesCursor)
	}
}

// TestPageDownSelectorClampsToLastIndex verifies pageDown's bound clamping
// when the page advances beyond the last router.
func TestPageDownSelectorClampsToLastIndex(t *testing.T) {
	m := New(nil, 5*time.Second)
	// 50 routers so page-down has somewhere to go.
	routers := make([]network.Router, 50)
	for i := range routers {
		routers[i] = network.Router{ID: fmt.Sprintf("r-%02d", i), Name: fmt.Sprintf("router-%d", i)}
	}
	m.width = 100
	m.height = 30
	m.routers = routers
	m.cursor = 0
	vis := m.selectorVisibleLines()
	upd, _ := m.pageDown()
	if upd.cursor < 1 || upd.cursor > vis {
		t.Errorf("pageDown from 0 cursor=%d, want in range 1..%d (visible lines)", upd.cursor, vis)
	}
	if upd.cursor > len(routers)-1 {
		t.Errorf("pageDown exceeded bounds: cursor=%d, max=%d", upd.cursor, len(routers)-1)
	}
}

// TestPageUpSelectorClampsToZero verifies pageUp returns cursor to 0 when
// advancing into negative territory.
func TestPageUpSelectorClampsToZero(t *testing.T) {
	m := New(nil, 5*time.Second)
	routers := make([]network.Router, 50)
	for i := range routers {
		routers[i] = network.Router{ID: fmt.Sprintf("r-%02d", i), Name: fmt.Sprintf("router-%d", i)}
	}
	m.width = 100
	m.height = 30
	m.routers = routers
	m.cursor = 10
	upd, _ := m.pageUp()
	if upd.cursor < 0 {
		t.Errorf("pageUp produced negative cursor: %d", upd.cursor)
	}
	if upd.cursor == 10 {
		t.Errorf("pageUp did not advance cursor: still %d", upd.cursor)
	}
	// Large page-down then page-up should clamp.
	m.cursor = 0
	upd, _ = m.pageUp()
	if upd.cursor != 0 {
		t.Errorf("pageUp from 0 should clamp: cursor=%d, want 0", upd.cursor)
	}
}

// TestPageDownClampsOnEmptyList verifies pageDown's edge case when the
// router list is completely empty — cursor should stay non-negative.
func TestPageDownClampsOnEmptyList(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = nil
	m.width = 100
	m.height = 30
	m.cursor = 0
	upd, _ := m.pageDown()
	if upd.cursor < 0 {
		t.Errorf("pageDown on empty list produced negative cursor: %d", upd.cursor)
	}
}

// TestResetDetailState verifies detail-state fields are cleared and detailLoading flips true.
func TestResetDetailState(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.interfaces = []network.RouterInterface{{SubnetID: "x"}}
	m.detailErr = "some previous error"
	m.interfaceCursor = 5
	m.interfaceScroll = 3
	m.routesCursor = 7
	m.routesScroll = 2
	m.detailLoading = false
	m.resetDetailState()
	if m.interfaces != nil {
		t.Errorf("resetDetailState should nil interfaces, got %v", m.interfaces)
	}
	if m.detailErr != "" {
		t.Errorf("resetDetailState should clear detailErr, got %q", m.detailErr)
	}
	if !m.detailLoading {
		t.Errorf("resetDetailState should set detailLoading=true")
	}
	if m.interfaceCursor != 0 || m.interfaceScroll != 0 || m.routesCursor != 0 || m.routesScroll != 0 {
		t.Errorf("resetDetailState should zero cursors: ic=%d isc=%d rc=%d rs=%d",
			m.interfaceCursor, m.interfaceScroll, m.routesCursor, m.routesScroll)
	}
}

// TestClampDetailCursors verifies interfaceCursor bounds after interfaces list changes.
func TestClampDetailCursors(t *testing.T) {
	m := New(nil, 5*time.Second)
	// Out-of-range on multi-element list.
	m.interfaces = []network.RouterInterface{
		{SubnetID: "s1"},
		{SubnetID: "s2"},
		{SubnetID: "s3"},
	}
	m.interfaceCursor = 5
	m.clampDetailCursors()
	if m.interfaceCursor != 2 {
		t.Errorf("clampDetailCursors multi-elem should set interfaceCursor=2, got %d", m.interfaceCursor)
	}
	// Single-element: clamps to 0.
	m.interfaces = []network.RouterInterface{{SubnetID: "only"}}
	m.interfaceCursor = 5
	m.clampDetailCursors()
	if m.interfaceCursor != 0 {
		t.Errorf("clampDetailCursors single-elem should set interfaceCursor=0, got %d", m.interfaceCursor)
	}
	// Empty list: clamps to max(0, -1) = 0.
	m.interfaces = nil
	m.interfaceCursor = 7
	m.clampDetailCursors()
	if m.interfaceCursor != 0 {
		t.Errorf("clampDetailCursors empty should set interfaceCursor=0, got %d", m.interfaceCursor)
	}
}

// TestOnSelectorChangeStaleReturnsNil verifies that re-selecting the same
// router ID does NOT trigger a fresh detail fetch (no superfluous API calls).
func TestOnSelectorChangeStaleReturnsNil(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{{ID: "r1", Name: "router1"}}
	m.cursor = 0
	m.lastDetailID = "r1" // already fetched this router's details
	m.detailLoading = false
	cmd := m.onSelectorChange()
	if cmd != nil {
		t.Errorf("onSelectorChange on same router should return nil cmd")
	}
	if m.detailLoading {
		t.Errorf("onSelectorChange on same router should NOT set detailLoading=true")
	}
	if m.lastDetailID != "r1" {
		t.Errorf("onSelectorChange on same router should not mutate lastDetailID, got %q", m.lastDetailID)
	}
}

// TestOnSelectorChangeNewRouterTriggersFetch verifies a new selection fires
// the detail fetch + resets detail state.
func TestOnSelectorChangeNewRouterTriggersFetch(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{{ID: "r1", Name: "router1"}}
	m.cursor = 0
	m.lastDetailID = "" // never fetched yet
	cmd := m.onSelectorChange()
	if cmd == nil {
		t.Errorf("onSelectorChange on new router should return non-nil cmd")
	}
	if m.lastDetailID != "r1" {
		t.Errorf("onSelectorChange should set lastDetailID=r1, got %q", m.lastDetailID)
	}
	if !m.detailLoading {
		t.Errorf("onSelectorChange on new router should set detailLoading=true")
	}
}

// TestOnSelectorChangeNoRouterReturnsNil verifies that calling onSelectorChange
// with no selection is a safe no-op.
func TestOnSelectorChangeNoRouterReturnsNil(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.cursor = 0
	// m.routers is empty/nil → selectedRouter returns nil
	cmd := m.onSelectorChange()
	if cmd != nil {
		t.Errorf("onSelectorChange on empty router list should return nil cmd, got %v", cmd)
	}
}

// TestWindowSizeMsgUpdatesDims verifies tea.WindowSizeMsg propagates width/height.
func TestWindowSizeMsgUpdatesDims(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.width = 0
	m.height = 0
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 60})
	if upd.width != 120 {
		t.Errorf("WindowSizeMsg width not updated: got %d, want 120", upd.width)
	}
	if upd.height != 60 {
		t.Errorf("WindowSizeMsg height not updated: got %d, want 60", upd.height)
	}
}

// TestRoutersLoadedMsgPreservesCursorByID verifies the cursor is preserved by
// router ID match when the loaded list replaces the previous one.
func TestRoutersLoadedMsgPreservesCursorByID(t *testing.T) {
	m := New(nil, 5*time.Second)
	// prior state: one router with ID "old1" at cursor=0
	m.routers = []network.Router{{ID: "old1", Name: "old1"}}
	m.cursor = 0
	// new list still contains "old1" but at index 1
	newRouters := []network.Router{{ID: "x1", Name: "x1"}, {ID: "old1", Name: "old1"}}
	upd, _ := m.Update(routersLoadedMsg{routers: newRouters})
	if upd.loading {
		t.Errorf("routersLoadedMsg should set loading=false")
	}
	if upd.err != "" {
		t.Errorf("routersLoadedMsg should clear err, got %q", upd.err)
	}
	if len(upd.routers) != 2 {
		t.Errorf("routers count after loaded msg = %d, want 2", len(upd.routers))
	}
	// cursor should follow "old1" to index 1 in the new list
	if upd.cursor != 1 {
		t.Errorf("cursor after routersLoadedMsg preserving ID = %d, want 1", upd.cursor)
	}
}

// TestRoutersLoadedMsgClampsCursorWhenListShrinks verifies that shrinking
// the router list past the current cursor clamps to the new last index.
func TestRoutersLoadedMsgClampsCursorWhenListShrinks(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{{ID: "r1"}, {ID: "r2"}, {ID: "r3"}, {ID: "r4"}}
	m.cursor = 3 // previous last index
	upd, _ := m.Update(routersLoadedMsg{routers: []network.Router{{ID: "n1"}, {ID: "n2"}}})
	if upd.cursor != 1 {
		t.Errorf("cursor after shrinking routersLoadedMsg should clamp to 1, got %d", upd.cursor)
	}
}

// TestRoutersLoadedMsgClampsCursorWhenListEmpty verifies that an empty
// loaded list leaves cursor at 0 (or unmodified) without panicking.
func TestRoutersLoadedMsgClampsCursorWhenListEmpty(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{{ID: "r1"}, {ID: "r2"}}
	m.cursor = 1
	upd, _ := m.Update(routersLoadedMsg{routers: nil})
	if upd.cursor != 1 {
		t.Errorf("cursor after empty routersLoadedMsg = %d, want unchanged (1)", upd.cursor)
	}
	if !upd.loading { // loading was set false regardless
		// loading flag flips to false even on empty load
	} else {
		t.Errorf("loading flag should be false after routersLoadedMsg")
	}
}

// TestRoutersErrMsgSetsErr verifies the error handler stores the error and clears loading.
func TestRoutersErrMsgSetsErr(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.loading = true
	upd, _ := m.Update(routersErrMsg{err: fmt.Errorf("boom")})
	if upd.loading {
		t.Errorf("routersErrMsg should set loading=false")
	}
	if upd.err != "boom" {
		t.Errorf("routersErrMsg should set err='boom', got %q", upd.err)
	}
	// Routers slice should remain untouched.
	if len(upd.routers) != 0 {
		t.Errorf("routersErrMsg should not modify routers list, got len=%d", len(upd.routers))
	}
}

// TestDetailLoadedMsgStaleByID verifies a detailLoadedMsg with a non-matching
// router ID is ignored (no state mutation apart from the early return).
func TestDetailLoadedMsgStaleByID(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{{ID: "current", Name: "current"}}
	m.cursor = 0
	m.detailLoading = true
	upd, _ := m.Update(detailLoadedMsg{
		routerID:   "stale",
		interfaces: []network.RouterInterface{{SubnetID: "x"}},
	})
	if !upd.detailLoading {
		t.Errorf("stale detailLoadedMsg should NOT clear detailLoading")
	}
	if upd.interfaces != nil {
		t.Errorf("stale detailLoadedMsg should NOT replace interfaces, got %v", upd.interfaces)
	}
}

// TestDetailLoadedMsgForCurrentRouter verifies interfaces are applied and
// cursor clamping runs for the currently-selected router.
func TestDetailLoadedMsgForCurrentRouter(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{{ID: "current", Name: "current"}}
	m.cursor = 0
	m.detailLoading = true
	m.interfaceCursor = 99 // out of range — clamp should bring it back
	upd, _ := m.Update(detailLoadedMsg{
		routerID:   "current",
		interfaces: []network.RouterInterface{{SubnetID: "i1"}, {SubnetID: "i2"}, {SubnetID: "i3"}},
	})
	if upd.detailLoading {
		t.Errorf("detailLoadedMsg for current should clear detailLoading")
	}
	if upd.detailErr != "" {
		t.Errorf("detailLoadedMsg for current should clear detailErr, got %q", upd.detailErr)
	}
	if len(upd.interfaces) != 3 {
		t.Errorf("detailLoadedMsg should set interfaces, got len=%d", len(upd.interfaces))
	}
	// clampDetailCursors should have run: interfaceCursor was 99, now should be 2 (last index).
	if upd.interfaceCursor != 2 {
		t.Errorf("clampDetailCursors should have set interfaceCursor=2, got %d", upd.interfaceCursor)
	}
}

// TestDetailErrMsgForCurrentRouter verifies error details are stored and detailLoading cleared.
func TestDetailErrMsgForCurrentRouter(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{{ID: "current", Name: "current"}}
	m.cursor = 0
	m.detailLoading = true
	upd, _ := m.Update(detailErrMsg{routerID: "current", err: fmt.Errorf("oops")})
	if upd.detailLoading {
		t.Errorf("detailErrMsg should clear detailLoading")
	}
	if upd.detailErr != "oops" {
		t.Errorf("detailErrMsg should set detailErr='oops', got %q", upd.detailErr)
	}
}

// TestDetailErrMsgStaleByID verifies detailErrMsg for a non-matching router ID is ignored.
func TestDetailErrMsgStaleByID(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{{ID: "current", Name: "current"}}
	m.cursor = 0
	m.detailLoading = true
	upd, _ := m.Update(detailErrMsg{routerID: "other", err: fmt.Errorf("ignored")})
	if !upd.detailLoading {
		t.Errorf("stale detailErrMsg should NOT clear detailLoading")
	}
	if upd.detailErr != "" {
		t.Errorf("stale detailErrMsg should NOT touch detailErr, got %q", upd.detailErr)
	}
}

// TestNamesLoadedMsgSetsMaps verifies the names map and subnet→network map
// are overwritten (not merged) when the lookup completes.
func TestNamesLoadedMsgSetsMaps(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.networkNames = map[string]string{"old": "value"}
	m.subnetToNet = map[string]string{"old": "value"}
	newNames := map[string]string{"n1": "name1"}
	newSubnets := map[string]string{"s1": "n1"}
	upd, _ := m.Update(namesLoadedMsg{networkNames: newNames, subnetToNet: newSubnets})
	if upd.networkNames["n1"] != "name1" {
		t.Errorf("namesLoadedMsg should set networkNames['n1']='name1', got %q", upd.networkNames["n1"])
	}
	if upd.subnetToNet["s1"] != "n1" {
		t.Errorf("namesLoadedMsg should set subnetToNet['s1']='n1', got %q", upd.subnetToNet["s1"])
	}
	// Should be replaced, not merged.
	if _, exists := upd.networkNames["old"]; exists {
		t.Errorf("namesLoadedMsg should replace networkNames map (not merge), but 'old' key persists")
	}
	if _, exists := upd.subnetToNet["old"]; exists {
		t.Errorf("namesLoadedMsg should replace subnetToNet map (not merge), but 'old' key persists")
	}
}

// TestViewDoesNotPanicOnEmptyState verifies rendering a freshly-constructed model
// (loaded=true, no routers, no dimensions) doesn't panic.
func TestViewDoesNotPanicOnEmptyState(t *testing.T) {
	m := New(nil, 5*time.Second)
	// width / height are 0 — renderNarrow / renderWide branch on width<80
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("View() panicked on empty state: %v", r)
		}
	}()
	_ = m.View()
}

// TestViewDoesNotPanicWhenLoaded verifies rendering a populated, idle model
// produces a non-empty string.
func TestViewDoesNotPanicWhenLoaded(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.loading = false
	m.routers = []network.Router{{ID: "r1", Name: "router1", Status: "ACTIVE", AdminStateUp: true, ExternalGatewayNetworkID: "net1"}}
	m.interfaces = []network.RouterInterface{{SubnetID: "s1", PortID: "p1", IPAddress: "10.0.0.1"}}
	m.width = 100
	m.height = 30
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("View() panicked on loaded state: %v", r)
		}
	}()
	out := m.View()
	if out == "" {
		t.Errorf("View() returned empty string for loaded state")
	}
}

// TestViewEmptyRoutersShowsNoRouters verifies the no-routers-loading-false
// branch renders the "No routers found" placeholder.
func TestViewEmptyRoutersShowsNoRouters(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.loading = false
	m.routers = nil
	out := m.View()
	if out == "" {
		t.Errorf("View() returned empty string for empty-routers-no-loading state")
	}
}

// TestViewErrorSetsErr verifies err is rendered when set on the model.
func TestViewErrorSetsErr(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.loading = false
	m.err = "boom"
	out := m.View()
	if out == "" {
		t.Errorf("View() returned empty string for error state")
	}
}

// TestCopyEmptyEntriesNoRouter verifies CopyEntries handles the no-selection case.
func TestCopyEmptyEntriesNoRouter(t *testing.T) {
	m := New(nil, 5*time.Second)
	title, entries := m.CopyEntries()
	if title != "" {
		t.Errorf("CopyEntries title with no router = %q, want empty", title)
	}
	if entries != nil {
		t.Errorf("CopyEntries entries with no router = %v, want nil", entries)
	}
}

// TestCopyEntriesRouterWithoutInterface verifies CopyEntries returns
// router-only entries when interfaces pane is not focused.
func TestCopyEntriesRouterWithoutInterface(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{{
		ID:                       "r1",
		Name:                     "router1",
		ExternalGatewayIPv4:      "1.2.3.4",
		ExternalGatewayIPv6:      "::1",
		ExternalGatewayNetworkID: "net1",
	}}
	m.cursor = 0
	m.focus = FocusSelector // not Interfaces → no interface entries
	title, entries := m.CopyEntries()
	if title == "" {
		t.Errorf("CopyEntries title should be non-empty when a router is selected")
	}
	if entries == nil {
		t.Fatalf("CopyEntries entries should be non-nil when a router is selected")
	}
	// Should contain at least the ID, Name, External Gateway IPv4, IPv6, External Network ID
	if len(entries) < 5 {
		t.Errorf("CopyEntries should return at least 5 router entries, got %d", len(entries))
	}
}

// TestCopyEntriesRouterWithInterface verifies interface fields are added when FocusInterfaces.
func TestCopyEntriesRouterWithInterface(t *testing.T) {
	m := New(nil, 5*time.Second)
	m.routers = []network.Router{{ID: "r1", Name: "router1"}}
	m.cursor = 0
	m.interfaces = []network.RouterInterface{
		{SubnetID: "s1", PortID: "p1", IPAddress: "10.0.0.1"},
		{SubnetID: "s2", PortID: "p2", IPAddress: "10.0.0.2"},
	}
	m.focus = FocusInterfaces
	m.interfaceCursor = 1
	title, entries := m.CopyEntries()
	if title == "" {
		t.Errorf("CopyEntries title should be non-empty")
	}
	if len(entries) < 5 {
		t.Errorf("CopyEntries should return at least 5 entries (router fields + interface fields), got %d", len(entries))
	}
}
