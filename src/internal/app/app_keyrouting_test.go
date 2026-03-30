package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/cloud"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
)

func TestServerDetailCtrlAOpensVolumePicker(t *testing.T) {
	m := newTestModel("dev", false)
	m.client = &cloud.Client{}
	m.view = viewServerDetail
	m.serverDetail = testServerDetailWithServer("srv-1", "srv-1")

	res, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Mod: tea.ModCtrl}))
	updated := res.(Model)
	if !updated.volumePicker.Active {
		t.Fatalf("volume picker should be active after ctrl+a on server detail")
	}
}

func TestServerDetailCtrlBOpensFIPPicker(t *testing.T) {
	m := newTestModel("dev", false)
	m.client = &cloud.Client{}
	m.view = viewServerDetail
	m.serverDetail = testServerDetailWithServer("srv-1", "srv-1")

	res, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'b', Mod: tea.ModCtrl}))
	updated := res.(Model)
	if !updated.fipPicker.Active {
		t.Fatalf("fip picker should be active after ctrl+b on server detail")
	}
}

func testServerDetailWithServer(id, name string) serverdetail.Model {
	d := serverdetail.New(nil, nil, nil, id, 5*time.Second)
	d.SetServer(&compute.Server{ID: id, Name: name})
	return d
}
