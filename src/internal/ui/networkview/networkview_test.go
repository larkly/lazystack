package networkview

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"fmt"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoadPreservesSelectionAndDetailLifecycle(t *testing.T) {
	m := New(nil, time.Second)
	m.networks = []network.Network{{ID: "b", Name: "Beta"}}
	m.lastDetailNetID = "b"
	m, cmd := m.Update(networksLoadedMsg{networks: []network.Network{{ID: "a", Name: "Alpha"}, {ID: "b", Name: "Beta"}}})
	if m.loading || m.cursor != 1 || cmd != nil {
		t.Fatal("refresh lost selection or redundantly fetched detail")
	}
	m.ports = []network.Port{{ID: "old"}}
	m.portsCursor = 4
	m, cmd = m.Update(networksLoadedMsg{networks: []network.Network{{ID: "a", Name: "Alpha", SubnetIDs: []string{"s", "missing"}}}, allSubnets: map[string]network.Subnet{"s": {ID: "s", Name: "Subnet", CIDR: "10.0.0.0/24"}}})
	if m.cursor != 0 || m.lastDetailNetID != "a" || !m.detailLoading || len(m.ports) != 0 || cmd == nil {
		t.Fatal("changed selection did not reset/fetch detail")
	}
	m, _ = m.Update(detailLoadedMsg{netID: "b", ports: []network.Port{{ID: "stale"}}})
	m, _ = m.Update(detailErrMsg{netID: "b", err: errors.New("stale")})
	if len(m.ports) != 0 || m.detailErr != "" || !m.detailLoading {
		t.Fatal("stale detail applied")
	}
	m.portsCursor = 8
	m.subnetCursor = 8
	m, _ = m.Update(detailLoadedMsg{netID: "a", ports: []network.Port{{ID: "p", Name: "Port"}}, serverNames: map[string]string{"vm": "VM"}, sgNames: map[string]string{"sg": "Default"}})
	if m.detailLoading || m.portsCursor != 0 || m.subnetCursor != 0 || m.SGNames()["sg"] != "Default" {
		t.Fatal("detail not applied/clamped")
	}
	m.focus = FocusSubnets
	if m.SelectedSubnetID() != "s" || m.SelectedSubnetName() != "Subnet" || len(m.NetworkSubnets()) != 1 {
		t.Fatal("subnet lookup/focus")
	}
	m.focus = focusPorts
	if m.SelectedPortID() != "p" || m.SelectedSubnet() != nil {
		t.Fatal("port selection/focus")
	}
	title, entries := m.CopyEntries()
	if !strings.Contains(title, "Alpha") || len(entries) < 3 {
		t.Fatal("missing copy fields")
	}
	for _, width := range []int{60, 140} {
		m.SetSize(width, 60)
		view := m.View()
		if !strings.Contains(view, "Alpha") || !strings.Contains(view, "Ports") {
			t.Fatal(view)
		}
	}
	m, _ = m.Update(detailErrMsg{netID: "a", err: errors.New("detail failed")})
	if m.detailLoading || m.detailErr != "detail failed" {
		t.Fatal("detail error")
	}
	m, _ = m.Update(networksErrMsg{err: errors.New("network failed")})
	if !strings.Contains(m.View(), "network failed") {
		t.Fatal("error not rendered")
	}
	m, _ = m.Update(networksLoadedMsg{})
	if !strings.Contains(m.View(), "No networks found") {
		t.Fatal("empty not rendered")
	}
}

func TestKeyboardNavigationAndPendingHighlightFetch(t *testing.T) {
	m := New(nil, time.Second)
	m.loading = false
	m.SetSize(120, 25)
	for i := 0; i < 15; i++ {
		m.networks = append(m.networks, network.Network{ID: fmt.Sprint(i), Name: fmt.Sprintf("net-%d", i)})
	}
	m.lastDetailNetID = "0"
	m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	if m.cursor != 10 || m.selectorScroll == 0 || cmd == nil || m.lastDetailNetID != "10" {
		t.Fatal("page navigation")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	if m.cursor != 0 || m.selectorScroll != 0 {
		t.Fatal("page up")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	if !m.InPorts() {
		t.Fatal("backwards focus wrap")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if m.FocusedPane() != FocusSelector {
		t.Fatal("forward focus wrap")
	}
	m.ScrollToNames([]string{"net-14"})
	if m.SelectedNetworkID() != "14" || m.pendingDetailID != "14" || !m.detailLoading {
		t.Fatal("highlight navigation not scheduled")
	}
	m, cmd = m.Update(detailLoadedMsg{netID: "0"})
	if cmd == nil || m.pendingDetailID != "" || len(m.ports) != 0 {
		t.Fatal("stale completion did not flush pending fetch")
	}
	m.pendingDetailID = "14"
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if m.pendingDetailID != "" || m.SelectedNetworkID() != "13" {
		t.Fatal("manual navigation did not supersede pending fetch")
	}
	m.loading = true
	if _, cmd = m.Update(shared.TickMsg{}); cmd != nil {
		t.Fatal("tick while loading")
	}
	m.loading = false
	m.detailLoading = true
	_, cmd = m.Update(shared.TickMsg{})
	if cmd == nil {
		t.Fatal("list refresh should continue during detail fetch")
	}
	if m.ForceRefresh() == nil || !m.loading || !m.detailLoading {
		t.Fatal("manual refresh state")
	}
}

func TestFetchOperationsHTTP(t *testing.T) {
	for _, fail := range []string{"", "/networks", "/subnets", "/ports", "/security-groups"} {
		t.Run("fail="+fail, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == fail {
					http.Error(w, "failed", 500)
					return
				}
				switch r.URL.Path {
				case "/networks":
					fmt.Fprint(w, `{"networks":[{"id":"n","name":"Network","router:external":true}]}`)
				case "/subnets":
					fmt.Fprint(w, `{"subnets":[{"id":"s","network_id":"n","cidr":"10.0.0.0/24"}]}`)
				case "/ports":
					if r.URL.Query().Get("network_id") != "n" {
						t.Error("missing network filter")
					}
					fmt.Fprint(w, `{"ports":[{"id":"z","device_owner":"network:dhcp","mac_address":"bb"},{"id":"b","device_owner":"compute:nova","mac_address":"bb","security_groups":["sg"]},{"id":"a","device_owner":"compute:nova","mac_address":"aa"}]}`)
				case "/security-groups":
					fmt.Fprint(w, `{"security_groups":[{"id":"sg","name":"Default"},{"id":"unused","name":"Other"}]}`)
				default:
					t.Errorf("unexpected path %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			m := New(&gophercloud.ServiceClient{ProviderClient: &gophercloud.ProviderClient{}, Endpoint: server.URL + "/"}, time.Second)
			msg := m.fetchNetworks()()
			if fail == "/networks" || fail == "/subnets" {
				if _, ok := msg.(networksErrMsg); !ok {
					t.Fatalf("want list error, got %#v", msg)
				}
			} else {
				got, ok := msg.(networksLoadedMsg)
				if !ok || len(got.networks) != 1 || got.allSubnets["s"].NetworkID != "n" || !got.externalIDs["n"] {
					t.Fatalf("list result %#v", msg)
				}
			}
			msg = m.fetchDetail("n")()
			if fail == "/ports" {
				e, ok := msg.(detailErrMsg)
				if !ok || e.netID != "n" {
					t.Fatalf("want scoped detail error %#v", msg)
				}
				return
			}
			got, ok := msg.(detailLoadedMsg)
			if !ok || got.netID != "n" || len(got.ports) != 3 || got.ports[0].ID != "a" || got.ports[1].ID != "b" || got.ports[2].ID != "z" {
				t.Fatalf("sorted detail %#v", msg)
			}
			if fail == "/security-groups" {
				if len(got.sgNames) != 0 {
					t.Fatal("failed optional SG lookup")
				}
			} else if got.sgNames["sg"] != "Default" || len(got.sgNames) != 1 {
				t.Fatal("SG names not filtered")
			}
		})
	}
}
