package app

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/modal"
	"github.com/larkly/lazystack/internal/ui/serverlist"
)

func TestServerTabReappliesSizeAfterInactiveResize(t *testing.T) {
	m, _ := listFixture(t)
	m.serverList.SetSize(m.width, m.height)
	m, _ = m.switchTab(1)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 170, Height: 60})
	m = next.(Model)
	m, cmd := m.switchTab(0)
	if cmd != nil || m.view != viewServerList {
		t.Fatal("server tab should restore list without refetch")
	}
	child := reflect.ValueOf(m.serverList)
	if child.FieldByName("width").Int() != 170 || child.FieldByName("height").Int() != 60 {
		t.Fatalf("server list retained stale size %dx%d", child.FieldByName("width").Int(), child.FieldByName("height").Int())
	}
}

func TestViewChangeListAndCreateTargets(t *testing.T) {
	cases := []struct {
		target, tab string
		view        activeView
		status      string
	}{
		{"routerlist", "routers", viewRouterView, "routerview"},
		{"keypairlist", "keypairs", viewKeypairList, "keypairlist"},
		{"lblist", "loadbalancers", viewLBView, "lbview"},
		{"imagelist", "images", viewImageView, "imageview"},
		{"secgroupview", "secgroups", viewSecGroupView, "secgroupview"},
		{"volumecreate", "volumes", viewVolumeCreate, "volumecreate"},
		{"keypaircreate", "keypairs", viewKeypairCreate, "keypaircreate"},
		{"servercreate", "servers", viewServerCreate, "servercreate"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			m, _ := listFixture(t)
			m.tabs = []TabDef{{Key: "servers"}, {Key: tc.tab}}
			m, _ = m.switchTab(1)
			m.returnToView = viewServerDetail
			next, cmd := m.handleViewChange(shared.ViewChangeMsg{View: tc.target})
			if next.view != tc.view || next.statusBar.CurrentView != tc.status || next.statusBar.Hint == "" {
				t.Fatalf("view=%v status=%q hint=%q", next.view, next.statusBar.CurrentView, next.statusBar.Hint)
			}
			if tc.target == "secgroupview" && next.returnToView != 0 {
				t.Fatal("security group navigation retained return origin")
			}
			// Create form Init may be nil; list refresh commands must execute real requests.
			if tc.target == "routerlist" || tc.target == "keypairlist" || tc.target == "lblist" || tc.target == "imagelist" || tc.target == "secgroupview" {
				if cmd == nil {
					t.Fatal("missing refresh")
				}
				commandMessages(cmd)
			}
		})
	}
}

func TestNestedDetailNavigationAndCrossResourceReturn(t *testing.T) {
	m, _ := listFixture(t)
	m.view = viewServerDetail
	m.activeTab = 0
	m, cmd := m.handleDetailNavigation(shared.NavigateToDetailMsg{Resource: "volume", ID: "vol"})
	if m.view != viewVolumeDetail || m.nav.Len() != 1 || m.nav.TopView() != viewServerDetail {
		t.Fatal("volume detail did not push origin")
	}
	for _, msg := range commandMessages(cmd) {
		m.volumeDetail, _ = m.volumeDetail.Update(msg)
	}
	m, cmd = m.handleViewChange(shared.ViewChangeMsg{View: "volumelist"})
	if m.view != viewServerDetail || m.nav.Len() != 0 {
		t.Fatal("volume back did not restore server")
	}
	commandMessages(cmd)
	m, cmd = m.handleResourceNavigation(shared.NavigateToResourceMsg{Tab: "volumes", Highlight: []string{"vol"}})
	if m.view != viewVolumeList || m.returnToView != viewServerDetail || m.activeTab != 1 {
		t.Fatal("resource jump lost origin")
	}
	commandMessages(cmd)
	m, cmd = m.handleViewChange(shared.ViewChangeMsg{View: "volumelist"})
	if m.view != viewServerDetail || m.returnToView != 0 || cmd != nil {
		t.Fatal("resource back did not consume returnToView")
	}
	m.view = viewImageView
	m, cmd = m.handleDetailNavigation(shared.NavigateToDetailMsg{Resource: "server", ID: "srv"})
	if m.view != viewServerDetail || m.returnToView != viewImageView {
		t.Fatal("server jump lost image origin")
	}
	commandMessages(cmd)
	for _, resource := range []string{"unknown"} {
		next, cmd := m.handleDetailNavigation(shared.NavigateToDetailMsg{Resource: resource, ID: "id"})
		if next.view != m.view || cmd != nil {
			t.Fatal("unknown resource should be inert")
		}
	}
	next, cmd := m.handleResourceNavigation(shared.NavigateToResourceMsg{Tab: "unknown"})
	if next.view != m.view || cmd != nil {
		t.Fatal("unknown tab should be inert")
	}
}

func TestServerConfirmModalCapturesSelectionAndBulkClearsIt(t *testing.T) {
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/servers/detail":
			fmt.Fprint(w, `{"servers":[{"id":"a","name":"alpha","status":"ACTIVE","os-extended-volumes:volumes_attached":[{"id":"vol"}]},{"id":"b","name":"beta","status":"SHUTOFF"}]}`)
		case "/flavors/detail":
			fmt.Fprint(w, `{"flavors":[]}`)
		case "/images":
			fmt.Fprint(w, `{"images":[]}`)
		default:
			w.WriteHeader(202)
		}
	})
	// Use the regular server-list loader rather than injecting its private selection.
	m.serverList = serverlist.New(m.client.Compute, m.client.Image, m.refreshInterval)
	for _, msg := range commandMessages(m.serverList.Init()) {
		m.serverList, _ = m.serverList.Update(msg)
	}
	m.view = viewServerList
	single, _ := m.openDeleteConfirm()
	detail, detailCmd := m.handleViewChange(shared.ViewChangeMsg{View: "serverdetail"})
	if detail.view != viewServerDetail || detail.serverDetail.ServerID() != "a" || detailCmd == nil || detail.statusBar.CurrentView != "serverdetail" {
		t.Fatal("selected server did not open its detail view")
	}
	if single.activeModal != modalConfirm || single.confirm.ServerID != "a" || len(single.confirm.VolumeIDs) != 1 || single.confirm.VolumeIDs[0] != "vol" {
		t.Fatalf("single confirm=%+v", single.confirm)
	}
	m.serverList, _ = m.serverList.Update(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
	m.serverList, _ = m.serverList.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	m.serverList, _ = m.serverList.Update(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
	if m.serverList.SelectionCount() != 2 {
		t.Fatalf("selection=%d", m.serverList.SelectionCount())
	}
	next, _ := m.openToggleConfirm("stop/start")
	if next.activeModal != modalConfirm || len(next.confirm.Servers) != 2 {
		t.Fatalf("bulk confirm=%+v", next.confirm)
	}
	counts := countBulkActions(next.confirm.Servers, next.confirm.Action)
	if counts["stop"] != 1 || counts["start"] != 1 {
		t.Fatalf("resolved toggles=%v", counts)
	}
	next, cmd := next.executeAction(modal.ConfirmAction{Action: next.confirm.Action, Servers: next.confirm.Servers})
	if next.serverList.SelectionCount() != 0 {
		t.Fatal("bulk execute did not clear selection")
	}
	if r, ok := cmd().(bulkResultMsg); !ok || len(r.failed) != 0 || len(r.succeeded) != 2 {
		t.Fatal("bulk action failed")
	}
}

func TestNavigationAdditionalTargets(t *testing.T) {
	m, _ := listFixture(t)
	for _, tab := range []string{"secgroups", "networks"} {
		m.view = viewServerDetail
		next, cmd := m.handleResourceNavigation(shared.NavigateToResourceMsg{Tab: tab, Highlight: []string{"target"}})
		if next.returnToView != viewServerDetail || next.view == viewServerDetail || cmd == nil {
			t.Fatalf("resource jump to %s failed", tab)
		}
		commandMessages(cmd)
	}
	m.client.ProviderClient = &gophercloud.ProviderClient{EndpointLocator: func(gophercloud.EndpointOpts) (string, error) { return m.client.Compute.Endpoint, nil }}
	m, cmd := m.handleViewChange(shared.ViewChangeMsg{View: "servicecatalog"})
	if m.view != viewServiceCatalog || m.nav.TopView() != viewServerDetail || cmd == nil || m.statusBar.CurrentView != "servicecatalog" {
		t.Fatal("catalog navigation did not capture origin")
	}
	for _, msg := range commandMessages(cmd) {
		m.serviceCatalog, _ = m.serviceCatalog.Update(msg)
	}
	if !strings.Contains(m.serviceCatalog.View(), "Compute") {
		t.Fatal("catalog command did not populate service entries")
	}
	m.tabs = []TabDef{{Key: "servers"}, {Key: "unknown"}}
	next, cmd := m.switchTab(1)
	if cmd != nil || next.view != m.view {
		t.Fatal("unknown tab must not replace active view")
	}
}
