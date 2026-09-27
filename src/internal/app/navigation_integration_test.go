package app

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/actionlog"
	"github.com/larkly/lazystack/internal/ui/consolelog"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
	"github.com/larkly/lazystack/internal/ui/serverlist"
	"github.com/larkly/lazystack/internal/ui/volumedetail"
	"github.com/larkly/lazystack/internal/ui/volumelist"
)

// Drain the finite command tree, not messages' recurring spinner/ticker commands.
func commandMessages(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, commandMessages(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func listFixture(t *testing.T) (Model, *[]string) {
	t.Helper()
	requests := new([]string)
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r.URL.Path)
		if r.Method != "GET" && !(r.Method == "POST" && r.URL.Path == "/servers/srv/action") {
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		responses := map[string]string{
			"/servers/srv/action": `{"output":"fixture console"}`, "/servers/srv/os-instance-actions": `{"instanceActions":[]}`,
			"/servers/detail": `{"servers":[]}`, "/servers": `{"servers":[]}`, "/flavors/detail": `{"flavors":[]}`, "/images": `{"images":[]}`, "/volumes/detail": `{"volumes":[]}`, "/floatingips": `{"floatingips":[]}`, "/security-groups": `{"security_groups":[]}`, "/networks": `{"networks":[]}`, "/subnets": `{"subnets":[]}`, "/ports": `{"ports":[]}`, "/lbaas/loadbalancers": `{"loadbalancers":[]}`, "/routers": `{"routers":[]}`, "/os-keypairs": `{"keypairs":[]}`, "/zones": `{"zones":[]}`,
			"/servers/srv": `{"server":{"id":"srv","name":"fixture-server","status":"ACTIVE","flavor":{"id":"flavor"}}}`, "/flavors/flavor": `{"flavor":{"id":"flavor","name":"tiny"}}`, "/servers/srv/os-volume_attachments": `{"volumeAttachments":[]}`, "/volumes/vol": `{"volume":{"id":"vol","name":"fixture-volume","status":"available"}}`,
		}
		if body, ok := responses[r.URL.Path]; ok {
			fmt.Fprint(w, body)
		} else {
			t.Errorf("unexpected endpoint %s", r.URL)
			http.Error(w, "unhandled fixture endpoint", 404)
		}
	})
	m.serverList = serverlist.New(m.client.Compute, m.client.Image, time.Hour)
	m.volumeList = volumelist.New(m.client.BlockStorage, m.client.Compute, time.Hour)
	m.serverDetail = serverdetail.New(m.client.Compute, m.client.Network, m.client.BlockStorage, "srv", time.Hour)
	m.volumeDetail = volumedetail.New(m.client.BlockStorage, m.client.Compute, "vol")
	m.consoleLog = consolelog.New(m.client.Compute, "srv", "fixture-server")
	m.actionLog = actionlog.New(m.client.Compute, "srv", "fixture-server")
	m.view = viewServerList
	m.tabs = DefaultTabs()
	m.tabInited = make([]bool, len(m.tabs))
	return m, requests
}

func TestSwitchTabLazyInitializationAndReactivation(t *testing.T) {
	cases := []struct {
		key                 string
		view                activeView
		status, path, field string
	}{
		{"volumes", viewVolumeList, "volumelist", "/volumes/detail", "volumeList"},
		{"floatingips", viewFloatingIPList, "floatingiplist", "/floatingips", "floatingIPList"},
		{"secgroups", viewSecGroupView, "secgroupview", "/security-groups", "secGroupView"},
		{"networks", viewNetworkList, "networkview", "/networks", "networkView"},
		{"loadbalancers", viewLBView, "lbview", "/lbaas/loadbalancers", "lbView"},
		{"routers", viewRouterView, "routerview", "/routers", "routerView"},
		{"keypairs", viewKeypairList, "keypairlist", "/os-keypairs", "keypairList"},
		{"images", viewImageView, "imageview", "/images", "imageView"},
		{"dns", viewDNSList, "dnslist", "/zones", "dnsList"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			m, requests := listFixture(t)
			m.tabs = []TabDef{{Key: "servers"}, {Key: tc.key}}
			for pass := 0; pass < 2; pass++ {
				m.activeTab = 0
				m.view = viewServerList
				var cmd tea.Cmd
				m, cmd = m.switchTab(1)
				if m.view != tc.view || m.activeTab != 1 || !m.tabInited[1] || m.statusBar.CurrentView != tc.status || m.statusBar.Hint == "" || cmd == nil {
					t.Fatalf("tab not initialized: view=%v status=%q cmd=%v", m.view, m.statusBar.CurrentView, cmd != nil)
				}
				before := len(*requests)
				for _, msg := range commandMessages(cmd) {
					m, _ = m.updateActiveView(msg)
				}
				found := false
				for _, p := range (*requests)[before:] {
					if p == tc.path {
						found = true
					}
				}
				if !found {
					t.Errorf("activation %d did not fetch %s: %v", pass, tc.path, (*requests)[before:])
				}
				// Reading dimensions does not mutate child internals. Verify propagation at
				// the parent/child boundary, independently of terminal rendering styles.
				child := reflect.ValueOf(m).FieldByName(tc.field)
				if got := child.FieldByName("width").Int(); got != int64(m.width) {
					t.Errorf("width=%d want=%d", got, m.width)
				}
				if got := child.FieldByName("height").Int(); got != int64(m.height) {
					t.Errorf("height=%d want=%d", got, m.height)
				}
				_, same := m.switchTab(1)
				if same != nil {
					t.Error("same top-level tab must not refetch")
				}
				// Resize while another tab is active; reactivation must reapply dimensions.
				m.activeTab = 0
				m.view = viewServerList
				updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 55})
				m = updated.(Model)
			}
		})
	}
}

func TestRestoreNavEntryBehavior(t *testing.T) {
	cases := []struct {
		view    activeView
		status  string
		refresh bool
	}{
		{viewServerList, "serverlist", true}, {viewServerDetail, "serverdetail", true},
		{viewVolumeList, "volumelist", true}, {viewVolumeDetail, "volumedetail", true},
		{viewConsoleLog, "consolelog", false}, {viewActionLog, "actionlog", false},
		{viewCloudPicker, "cloudpicker", false},
	}
	for _, tc := range cases {
		for _, tab := range []int{-1, 1, 999} {
			t.Run(fmt.Sprintf("%s/tab=%d", tc.status, tab), func(t *testing.T) {
				m, requests := listFixture(t)
				m.activeTab = 0
				next, cmd := m.restoreNavEntry(NavEntry{View: tc.view, Tab: tab})
				wantTab := 0
				if tab == 1 {
					wantTab = 1
				}
				if next.view != tc.view || next.activeTab != wantTab || next.statusBar.CurrentView != tc.status || (cmd != nil) != tc.refresh {
					t.Fatalf("restore view=%v tab=%d status=%s cmd=%v", next.view, next.activeTab, next.statusBar.CurrentView, cmd != nil)
				}
				msgs := commandMessages(cmd)
				if tc.view == viewServerList {
					if len(msgs) != 1 {
						t.Fatal(msgs)
					}
					if _, ok := msgs[0].(shared.RefreshServersMsg); !ok {
						t.Fatalf("refresh=%T", msgs[0])
					}
				}
				if tc.refresh && tc.view != viewServerList && len(*requests) == 0 {
					t.Fatal("restore command never fetched data")
				}
			})
		}
	}
	t.Run("missing server falls back", func(t *testing.T) {
		m, _ := listFixture(t)
		m.serverDetail = serverdetail.Model{}
		next, cmd := m.restoreNavEntry(NavEntry{View: viewServerDetail, Tab: 0})
		if next.view != viewServerList || next.statusBar.CurrentView != "serverlist" {
			t.Fatal("missing detail not guarded")
		}
		if _, ok := cmd().(shared.RefreshServersMsg); !ok {
			t.Fatal("missing refresh")
		}
	})
}

func TestViewChangeReturnToViewAndNavStack(t *testing.T) {
	for _, origin := range []activeView{viewServerDetail, viewSecGroupView, viewImageView} {
		t.Run(fmt.Sprint(origin), func(t *testing.T) {
			m, _ := listFixture(t)
			// Initialize real tab models so hints come from the originating view.
			if origin == viewSecGroupView {
				m, _ = m.switchTab(4)
			}
			if origin == viewImageView {
				m, _ = m.switchTab(2)
			}
			m.view = viewVolumeList
			m.returnToView = origin
			next, cmd := m.handleViewChange(shared.ViewChangeMsg{View: "serverlist"})
			if next.view != origin || next.returnToView != 0 || cmd != nil || next.statusBar.CurrentView != next.viewName() || next.statusBar.Hint == "" {
				t.Fatalf("return origin=%v got=%v return=%v cmd=%v", origin, next.view, next.returnToView, cmd != nil)
			}
		})
	}
	for _, source := range []activeView{viewConsoleLog, viewActionLog, viewVolumeDetail} {
		for _, stacked := range []bool{false, true} {
			t.Run(fmt.Sprintf("source=%d/stack=%v", source, stacked), func(t *testing.T) {
				m, _ := listFixture(t)
				m.view = source
				m.nav = nil
				target := "serverdetail"
				if source == viewVolumeDetail {
					target = "volumelist"
				}
				if stacked {
					m.pushNav(viewServerList, 0)
					m.pushNav(viewServerDetail, 2)
				}
				next, cmd := m.handleViewChange(shared.ViewChangeMsg{View: target})
				want := viewServerList
				if source == viewVolumeDetail {
					want = viewVolumeList
				}
				if stacked {
					want = viewServerDetail
				}
				if next.view != want || cmd == nil {
					t.Fatalf("view=%v want=%v command=%v", next.view, want, cmd != nil)
				}
				commandMessages(cmd)
				if stacked {
					if next.activeTab != 2 || next.nav.Len() != 1 || next.nav.TopView() != viewServerList {
						t.Fatal("did not pop exactly the top entry")
					}
				}
			})
		}
	}
	t.Run("volume cross resource return", func(t *testing.T) {
		m, _ := listFixture(t)
		m.view = viewVolumeList
		m.returnToView = viewServerDetail
		next, cmd := m.handleViewChange(shared.ViewChangeMsg{View: "volumelist"})
		if next.view != viewServerDetail || next.returnToView != 0 || cmd != nil {
			t.Fatal("lost cross-resource origin")
		}
	})
	t.Run("missing detail resets origin", func(t *testing.T) {
		m, _ := listFixture(t)
		m.serverDetail = serverdetail.Model{}
		m.returnToView = viewServerDetail
		next, cmd := m.handleViewChange(shared.ViewChangeMsg{View: "serverlist"})
		if next.view != viewServerList || next.returnToView != 0 {
			t.Fatal("stale return origin")
		}
		if _, ok := cmd().(shared.RefreshServersMsg); !ok {
			t.Fatal("missing refresh")
		}
	})
	t.Run("unknown and console targets no-op", func(t *testing.T) {
		m, _ := listFixture(t)
		for _, target := range []string{"unknown", "consolelog", "serverdetail"} {
			next, cmd := m.handleViewChange(shared.ViewChangeMsg{View: target})
			if next.view != m.view || cmd != nil {
				t.Errorf("%s should be no-op without a selection", target)
			}
		}
	})
}

func TestIdlePauseAndWakeConsumesFirstKey(t *testing.T) {
	m, requests := listFixture(t)
	m.idleTimeout = time.Minute
	m.lastActivity = time.Now().Add(-2 * time.Minute)
	m.refreshInterval = time.Millisecond
	next, cmd := m.Update(shared.TickMsg{})
	m = next.(Model)
	if !m.idlePaused || cmd != nil || !strings.Contains(m.statusBar.Hint, "Paused") {
		t.Fatal("expired idle timer did not pause")
	}
	next, cmd = m.Update(shared.TickMsg{})
	m = next.(Model)
	if !m.idlePaused || cmd != nil || len(*requests) != 0 {
		t.Fatal("paused ticks must not fetch or reschedule")
	}
	old := m.lastActivity
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	m = next.(Model)
	if m.idlePaused || !m.lastActivity.After(old) || m.statusBar.Hint != "" || cmd == nil {
		t.Fatal("wake did not reset pause state")
	}
	if _, ok := cmd().(refreshTickMsg); !ok {
		t.Fatal("first wake key must restart tick, not quit")
	}
	if len(*requests) != 0 {
		t.Fatal("wake should restart timer, not directly fetch")
	}
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	_ = next
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("second q should perform normal quit")
	}
}

func TestIdleTickRemainsActiveWhenDisabledRecentOrUninitialized(t *testing.T) {
	for _, mode := range []string{"disabled", "recent", "uninitialized"} {
		t.Run(mode, func(t *testing.T) {
			m, requests := listFixture(t)
			m.refreshInterval = time.Millisecond
			for _, msg := range commandMessages(m.serverList.Init()) {
				m.serverList, _ = m.serverList.Update(msg)
			}
			*requests = nil
			m.idleTimeout = time.Minute
			m.lastActivity = time.Now()
			switch mode {
			case "disabled":
				m.idleTimeout = 0
				m.lastActivity = time.Now().Add(-time.Hour)
			case "uninitialized":
				m.lastActivity = time.Time{}
			}
			next, cmd := m.Update(shared.TickMsg{})
			m = next.(Model)
			if m.idlePaused || cmd == nil {
				t.Fatal("valid activity incorrectly paused")
			}
			ticks := 0
			for _, msg := range commandMessages(cmd) {
				if _, ok := msg.(refreshTickMsg); ok {
					ticks++
				}
			}
			if ticks != 1 {
				t.Errorf("tick chain count=%d want=1", ticks)
			}
			if len(*requests) == 0 {
				t.Fatal("active tick did not refresh real service")
			}
		})
	}
}
