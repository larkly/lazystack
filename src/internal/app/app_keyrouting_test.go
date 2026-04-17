package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/cloud"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
	"github.com/larkly/lazystack/internal/ui/serverlist"
)

func TestServerDetailCtrlAOpensVolumePicker(t *testing.T) {
	m := newTestModel("dev", false)
	m.client = &cloud.Client{BlockStorage: &gophercloud.ServiceClient{}}
	m.view = viewServerDetail
	m.serverDetail = testServerDetailWithServer("srv-1", "srv-1")

	res, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))
	updated := res.(Model)
	if !updated.volumePicker.Active {
		t.Fatalf("volume picker should be active after ctrl+a on server detail")
	}
}

func TestServerDetailCtrlAWithoutBlockStorageIsGated(t *testing.T) {
	m := newTestModel("dev", false)
	m.client = &cloud.Client{}
	m.view = viewServerDetail
	m.serverDetail = testServerDetailWithServer("srv-1", "srv-1")

	res, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))
	updated := res.(Model)
	if updated.volumePicker.Active {
		t.Fatalf("volume picker should not open when block storage is unavailable")
	}
	if updated.statusBar.StickyHint == "" {
		t.Fatalf("expected a sticky hint explaining block storage is unavailable")
	}
}

func TestServerDetailCtrlUOpensFIPPicker(t *testing.T) {
	m := newTestModel("dev", false)
	m.client = &cloud.Client{}
	m.view = viewServerDetail
	m.serverDetail = testServerDetailWithServer("srv-1", "srv-1")

	res, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
	updated := res.(Model)
	if !updated.fipPicker.Active {
		t.Fatalf("fip picker should be active after ctrl+u on server detail")
	}
}

func TestServerListEscClearsFilterBeforeTabBackNav(t *testing.T) {
	m := newTestModel("dev", false)
	m.view = viewServerList
	m.returnToView = viewServerDetail
	m.serverDetail = testServerDetailWithServer("srv-1", "srv-1")
	m.serverList = testServerListFiltering("abc")
	if !m.serverList.IsFiltering() {
		t.Fatalf("test setup failed: server list should be in filtering mode")
	}

	res, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	updated := res.(Model)

	if updated.view != viewServerList {
		t.Fatalf("view = %v, want server list (esc should clear filter, not back-nav)", updated.view)
	}
	if updated.serverList.IsFiltering() {
		t.Fatalf("expected filtering mode to be off after esc")
	}
}

func testServerDetailWithServer(id, name string) serverdetail.Model {
	d := serverdetail.New(nil, nil, nil, id, 5*time.Second)
	d.SetServer(&compute.Server{ID: id, Name: name})
	return d
}

func testServerListFiltering(query string) serverlist.Model {
	l := serverlist.New(nil, nil, 5*time.Second)
	updated, _ := l.Update(tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	l = updated
	for _, r := range query {
		updated, _ = l.Update(tea.KeyPressMsg(tea.Key{Text: string(r), Code: r}))
		l = updated
	}
	return l
}
