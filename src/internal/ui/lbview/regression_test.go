package lbview

import (
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/loadbalancer"
	"testing"
	"time"
)

func TestLiveSearchResetsDetailForNewSelection(t *testing.T) {
	m := New(nil, time.Second)
	m.loading = false
	m.lbs = []loadbalancer.LoadBalancer{{ID: "a", Name: "Alpha"}, {ID: "b", Name: "Beta"}}
	m.lastDetailID = "a"
	m.listeners = []loadbalancer.Listener{{ID: "old"}}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'b', Text: "b"}))
	if !m.searchActive || m.searchFilter != "b" || m.LBID() != "b" || m.lastDetailID != "b" || !m.detailLoading || len(m.listeners) != 0 || cmd == nil {
		t.Fatalf("search selection retained old detail: filter=%q selected=%q detail=%q loading=%v listeners=%v", m.searchFilter, m.LBID(), m.lastDetailID, m.detailLoading, m.listeners)
	}
}

func TestDescendingSortKeepsEqualRowsStable(t *testing.T) {
	for col := range sortColumns {
		m := New(nil, time.Second)
		m.sortCol = col
		m.sortAsc = false
		m.lbs = []loadbalancer.LoadBalancer{{ID: "first", Name: "same", VipAddress: "same", ProvisioningStatus: "same", OperatingStatus: "same"}, {ID: "second", Name: "same", VipAddress: "same", ProvisioningStatus: "same", OperatingStatus: "same"}, {ID: "third", Name: "same", VipAddress: "same", ProvisioningStatus: "same", OperatingStatus: "same"}}
		m.sortLBs()
		if m.lbs[0].ID != "first" || m.lbs[1].ID != "second" || m.lbs[2].ID != "third" {
			t.Errorf("column %s reversed equal rows: %v", sortColumns[col], m.lbs)
		}
	}
}
