package app

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
	"github.com/larkly/lazystack/internal/ui/serverlist"
)

// navFixture serves read-only fixtures for the views used in navigation
// round trips.
func navFixture(t *testing.T) Model {
	t.Helper()
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		// Server detail loads its console log with a read-only POST action.
		if r.Method != http.MethodGet && !(r.Method == http.MethodPost && r.URL.Path == "/servers/srv/action") {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/servers/srv/action":
			fmt.Fprint(w, `{"output":""}`)
		case "/servers/detail":
			fmt.Fprint(w, `{"servers":[{"id":"srv","name":"web","status":"ACTIVE","flavor":{"id":"f"}}]}`)
		case "/volumes/detail":
			fmt.Fprint(w, `{"volumes":[{"id":"vol","name":"data","status":"available"}]}`)
		case "/volumes/vol":
			fmt.Fprint(w, `{"volume":{"id":"vol","name":"data","status":"available"}}`)
		case "/servers/srv":
			fmt.Fprint(w, `{"server":{"id":"srv","name":"web","status":"ACTIVE","flavor":{"id":"f"}}}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	})
	endpoint := m.client.Compute.Endpoint
	m.client.ProviderClient = &gophercloud.ProviderClient{
		EndpointLocator: func(gophercloud.EndpointOpts) (string, error) { return endpoint, nil },
	}
	m.serverList = serverlist.New(m.client.Compute, m.client.Image, time.Hour)
	m.serverList.SetConfig(m.configView.Cfg())
	for _, msg := range quickMessages(m.serverList.Init()) {
		m.serverList, _ = m.serverList.Update(msg)
	}
	m.view = viewServerList
	m.tabs = DefaultTabs()
	m.tabInited = make([]bool, len(m.tabs))
	return m
}

// step sends msg and follows navigation messages (view changes and detail
// jumps) that the resulting commands produce. Data fetches are delivered
// too so lists get populated.
func step(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, cmd := m.Update(msg)
	m = next.(Model)
	for _, out := range quickMessages(cmd) {
		switch out.(type) {
		case tea.QuitMsg:
			t.Fatal("unexpected quit")
		case nil:
			continue
		}
		m = step(t, m, out)
	}
	return m
}

func TestOverlayThenVolumeDetailReturnsToVolumes(t *testing.T) {
	m := navFixture(t)
	m = step(t, m, press("H"))
	if m.view != viewHypervisorList {
		t.Fatalf("H opened view %v", m.view)
	}
	m = step(t, m, press("esc"))
	if m.view != viewServerList || m.nav.Len() != 0 {
		t.Fatalf("esc from hypervisors: view=%v stack=%d", m.view, m.nav.Len())
	}
	m = step(t, m, press("2"))
	if m.view != viewVolumeList {
		t.Fatalf("tab 2 opened view %v", m.view)
	}
	m = step(t, m, press("enter"))
	if m.view != viewVolumeDetail {
		t.Fatalf("enter opened view %v", m.view)
	}
	m = step(t, m, press("esc"))
	if m.view != viewVolumeList || m.nav.Len() != 0 {
		t.Fatalf("esc from volume detail: view=%v stack=%d", m.view, m.nav.Len())
	}
}

func TestOverlayRoundTripsPreserveOrigin(t *testing.T) {
	overlays := map[string]activeView{
		"H": viewHypervisorList,
		"B": viewServiceCatalog,
		"U": viewUserManagement,
		"T": viewAuditLog,
	}
	origins := map[string]func(t *testing.T, m Model) Model{
		"server list": func(t *testing.T, m Model) Model { return m },
		"server detail": func(t *testing.T, m Model) Model {
			return step(t, m, press("enter"))
		},
		"volume list": func(t *testing.T, m Model) Model {
			return step(t, m, press("2"))
		},
	}
	for keyName, overlay := range overlays {
		for originName, open := range origins {
			if originName == "volume list" && (keyName == "U" || keyName == "T") {
				continue // server-only shortcuts
			}
			t.Run(keyName+" from "+originName, func(t *testing.T) {
				m := open(t, navFixture(t))
				origin, tab := m.view, m.activeTab
				m = step(t, m, press(keyName))
				if m.view != overlay {
					t.Fatalf("%s opened view %v, want %v", keyName, m.view, overlay)
				}
				m = step(t, m, press("esc"))
				if m.view != origin || m.activeTab != tab {
					t.Fatalf("returned to view %v tab %d, want %v tab %d", m.view, m.activeTab, origin, tab)
				}
				if m.nav.Len() != 0 {
					t.Fatalf("nav stack not balanced: %d entries left", m.nav.Len())
				}
				if m.statusBar.CurrentView != m.viewName() {
					t.Fatalf("status view %q, want %q", m.statusBar.CurrentView, m.viewName())
				}
			})
		}
	}
}

func TestReconnectClearsNavigationAndStaleDetails(t *testing.T) {
	m := navFixture(t)
	m = step(t, m, press("enter"))
	if m.view != viewServerDetail {
		t.Fatalf("setup: view %v", m.view)
	}
	m = step(t, m, press("H"))
	if m.nav.Len() == 0 {
		t.Fatal("setup: overlay should have pushed a nav entry")
	}
	m.returnToView = viewServerDetail
	next, _ := m.Update(shared.CloudConnectedMsg{ComputeClient: m.client.Compute})
	m = next.(Model)
	if m.nav.Len() != 0 {
		t.Fatalf("reconnect kept %d nav entries", m.nav.Len())
	}
	if m.serverDetail.ServerID() != "" {
		t.Fatal("reconnect kept the previous connection's server detail")
	}
	// Esc on the fresh server list cannot jump back into old state.
	m = step(t, m, press("esc"))
	if m.view != viewServerList {
		t.Fatalf("esc after reconnect restored stale view %v", m.view)
	}
	_ = serverdetail.Model{}
}
