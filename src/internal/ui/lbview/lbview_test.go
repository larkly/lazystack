package lbview

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/shared"
)

func TestListLoadStartsDetailAndResetsPreviousState(t *testing.T) {
	m := New(nil, time.Second)
	m.listeners = []loadbalancer.Listener{{ID: "old"}}
	m.listenerCursor = 4
	m, cmd := m.Update(lbsLoadedMsg{lbs: []loadbalancer.LoadBalancer{{ID: "lb", Name: "alpha"}}})
	if cmd == nil || m.loading || !m.detailLoading || m.lastDetailID != "lb" || len(m.listeners) != 0 || m.listenerCursor != 0 {
		t.Fatalf("list load did not transition into fresh detail: loading=%v detailLoading=%v detailID=%q listeners=%v cursor=%d cmd=%v", m.loading, m.detailLoading, m.lastDetailID, m.listeners, m.listenerCursor, cmd != nil)
	}
}

func TestSortColumnsPreserveSelection(t *testing.T) {
	for col := range sortColumns {
		for _, asc := range []bool{true, false} {
			m := New(nil, time.Second)
			m.lbs = []loadbalancer.LoadBalancer{{ID: "b", Name: "Zulu", VipAddress: "2", ProvisioningStatus: "PENDING_CREATE", OperatingStatus: "ONLINE"}, {ID: "a", Name: "alpha", VipAddress: "1", ProvisioningStatus: "ACTIVE", OperatingStatus: "OFFLINE"}}
			m.sortCol, m.sortAsc = col, asc
			m.sortLBs()
			want := "a"
			if !asc {
				want = "b"
			}
			if m.LBID() != want {
				t.Errorf("col=%d asc=%v first=%s want=%s", col, asc, m.LBID(), want)
			}
			selected := m.LBID()
			m, cmd := m.reverseSort()
			if m.LBID() != selected || !m.sortHighlight || cmd == nil {
				t.Fatal("reverse lost selected identity or highlight")
			}
			m, _ = m.cycleSort()
			if m.LBID() != selected || !m.sortAsc || m.sortCol != (col+1)%len(sortColumns) {
				t.Fatal("cycle lost selection or direction")
			}
			m, _ = m.Update(sortClearMsg{})
			if m.sortHighlight {
				t.Fatal("highlight not cleared")
			}
		}
	}
}

func TestAdaptivePolling(t *testing.T) {
	cases := []struct {
		prov, oper, mode string
		interval         time.Duration
	}{
		{"PENDING_UPDATE", "ERROR", "fast", 2 * time.Second}, {"ACTIVE", "ERROR", "medium", 5 * time.Second}, {"ACTIVE", "DEGRADED", "medium", 5 * time.Second}, {"ACTIVE", "ONLINE", "slow", 15 * time.Second}, {"ACTIVE", "OFFLINE", "medium", 10 * time.Second}, {"ACTIVE", "NO_MONITOR", "medium", 10 * time.Second}, {"ACTIVE", "unknown", "slow", 15 * time.Second}, {"ERROR", "unknown", "medium", 5 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.prov+tc.oper, func(t *testing.T) {
			m := New(nil, time.Second)
			m.lbs = []loadbalancer.LoadBalancer{{ID: "lb", ProvisioningStatus: tc.prov, OperatingStatus: tc.oper}}
			m, due := m.shouldRefreshDetail()
			if !due || m.pollMode != tc.mode || m.detailRefreshInterval != tc.interval {
				t.Fatalf("poll=%s %s due=%v", m.pollMode, m.detailRefreshInterval, due)
			}
			m.lastDetailFetch = time.Now().Add(time.Hour)
			if _, due = m.shouldRefreshDetail(); due {
				t.Fatal("fresh detail polled")
			}
			m.lastDetailFetch = time.Now().Add(-time.Hour)
			if _, due = m.shouldRefreshDetail(); !due {
				t.Fatal("expired detail not polled")
			}
		})
	}
	m := New(nil, time.Second)
	if _, due := m.shouldRefreshDetail(); due {
		t.Fatal("empty selector polled")
	}
	if _, cmd := m.Update(shared.TickMsg{}); cmd != nil {
		t.Fatal("tick while loading")
	}
	m.loading = false
	m.detailLoading = true
	if _, cmd := m.Update(shared.TickMsg{}); cmd != nil {
		t.Fatal("overlapping detail fetch")
	}
}

func TestDetailMessagesRejectStaleAndClampCursors(t *testing.T) {
	m := New(nil, time.Second)
	m.loading = false
	m.lbs = []loadbalancer.LoadBalancer{{ID: "lb", Name: "alpha"}}
	m.lastDetailID = "lb"
	m.detailLoading = true
	m.listeners = []loadbalancer.Listener{{ID: "old"}}
	m, _ = m.Update(detailLoadedMsg{lbID: "stale"})
	m, _ = m.Update(detailErrMsg{lbID: "stale", err: errors.New("stale")})
	if len(m.listeners) != 1 || !m.detailLoading || m.detailErr != "" {
		t.Fatal("stale response changed current detail")
	}
	m.listenerCursor = 9
	m.poolCursor = 9
	m.memberCursor = 9
	m, _ = m.Update(detailLoadedMsg{lbID: "lb", listeners: []loadbalancer.Listener{{ID: "listener"}}, pools: []loadbalancer.Pool{{ID: "pool"}}, members: map[string][]loadbalancer.Member{"pool": {{ID: "member"}}}})
	if m.detailLoading || m.SelectedListenerID() != "listener" || m.SelectedPoolID() != "pool" || m.SelectedMemberID() != "member" {
		t.Fatal("detail not installed/clamped")
	}
	m, _ = m.Update(detailErrMsg{lbID: "lb", err: errors.New("detail unavailable")})
	if m.detailErr != "detail unavailable" {
		t.Fatal(m.detailErr)
	}
	for _, width := range []int{60, 140} {
		m.SetSize(width, 60)
		if !strings.Contains(m.View(), "alpha") {
			t.Fatal("missing selected LB")
		}
	}
	m, _ = m.Update(lbsErrMsg{err: errors.New("list unavailable")})
	if !strings.Contains(m.View(), "list unavailable") {
		t.Fatal("missing list error")
	}
}

func TestNavigationSearchAndMemberSelection(t *testing.T) {
	m := New(nil, time.Second)
	m.loading = false
	m.SetSize(120, 30)
	m.lbs = []loadbalancer.LoadBalancer{{ID: "a", Name: "Alpha", VipAddress: "10.0.0.1"}, {ID: "b", Name: "Beta"}}
	m.lastDetailID = "a"
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.LBID() != "b" || m.lastDetailID != "b" || !m.detailLoading {
		t.Fatal("down did not select/fetch b")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	if m.focus != FocusMembers {
		t.Fatal("reverse focus did not wrap")
	}
	m.focus = FocusSelector
	m.searchFilter = "ALP"
	if len(m.visibleLBs()) != 1 {
		t.Fatal("case insensitive filter failed")
	}
	m.searchFilter = "10.0"
	if len(m.visibleLBs()) != 1 {
		t.Fatal("VIP filter failed")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.searchFilter != "" || m.cursor != 0 {
		t.Fatal("escape did not clear filter")
	}
	m.pools = []loadbalancer.Pool{{ID: "p"}, {ID: "q"}}
	m.members = map[string][]loadbalancer.Member{"p": {{ID: "one"}, {ID: "two"}}}
	m.ToggleMemberSelection()
	if m.SelectedMemberCount() != 1 {
		t.Fatal("toggle selection")
	}
	m.ToggleAllMemberSelection()
	if m.SelectedMemberCount() != 2 {
		t.Fatal("select all")
	}
	copy := m.SelectedPoolMembers()
	copy[0].ID = "changed"
	if m.SelectedMemberID() != "one" {
		t.Fatal("members accessor aliases backing array")
	}
	m.focus = FocusPools
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.poolCursor != 1 || len(m.selectedMembers) != 0 || m.memberCursor != 0 {
		t.Fatal("pool navigation retained member state")
	}
}

func TestFetchDetailHTTP(t *testing.T) {
	for _, fail := range []string{"", "/lbaas/loadbalancers/lb", "/lbaas/listeners", "/lbaas/pools", "/lbaas/pools/p/members", "/lbaas/healthmonitors/h"} {
		t.Run("fail="+fail, func(t *testing.T) {
			seen := map[string]bool{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen[r.URL.Path] = true
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == fail {
					http.Error(w, "unavailable", 500)
					return
				}
				responses := map[string]string{"/lbaas/loadbalancers/lb": `{"loadbalancer":{"id":"lb"}}`, "/lbaas/listeners": `{"listeners":[{"id":"l"}]}`, "/lbaas/pools": `{"pools":[{"id":"p","healthmonitor_id":"h"}]}`, "/lbaas/pools/p/members": `{"members":[{"id":"m"}]}`, "/lbaas/healthmonitors/h": `{"healthmonitor":{"id":"h"}}`}
				body, ok := responses[r.URL.Path]
				if !ok {
					t.Errorf("unexpected request %s", r.URL)
					http.NotFound(w, r)
					return
				}
				if r.URL.Path == "/lbaas/listeners" || r.URL.Path == "/lbaas/pools" {
					if r.URL.Query().Get("loadbalancer_id") != "lb" {
						t.Error("missing LB filter")
					}
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			m := New(&gophercloud.ServiceClient{ProviderClient: &gophercloud.ProviderClient{}, Endpoint: server.URL + "/"}, time.Second)
			msg := m.fetchDetail("lb")()
			if fail == "/lbaas/loadbalancers/lb" || fail == "/lbaas/listeners" || fail == "/lbaas/pools" {
				e, ok := msg.(detailErrMsg)
				if !ok || e.lbID != "lb" || e.err == nil {
					t.Fatalf("want scoped error, got %#v", msg)
				}
				return
			}
			got, ok := msg.(detailLoadedMsg)
			if !ok {
				t.Fatalf("got %#v", msg)
			}
			if got.lbID != "lb" || len(got.listeners) != 1 || len(got.pools) != 1 {
				t.Fatalf("incomplete detail %#v", got)
			}
			if (len(got.members["p"]) == 1) != (fail != "/lbaas/pools/p/members") || (got.monitors["h"] != nil) != (fail != "/lbaas/healthmonitors/h") {
				t.Fatal("optional detail failure handling")
			}
			if len(seen) != 5 {
				t.Fatalf("requests=%v", seen)
			}
		})
	}
}
