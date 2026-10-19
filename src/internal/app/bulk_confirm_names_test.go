package app

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/ui/serverlist"
)

// Every bulk server confirmation names the selected servers instead of
// asking about a server called "N servers".
func TestBulkServerConfirmationsNameServers(t *testing.T) {
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/servers/detail":
			fmt.Fprint(w, `{"servers":[{"id":"a","name":"alpha","status":"ACTIVE"},{"id":"b","name":"beta","status":"SHUTOFF"}]}`)
		case "/flavors/detail":
			fmt.Fprint(w, `{"flavors":[]}`)
		case "/images":
			fmt.Fprint(w, `{"images":[]}`)
		default:
			w.WriteHeader(202)
		}
	})
	m.serverList = serverlist.New(m.client.Compute, m.client.Image, m.refreshInterval)
	for _, msg := range commandMessages(m.serverList.Init()) {
		m.serverList, _ = m.serverList.Update(msg)
	}
	m.view = viewServerList
	m.serverList, _ = m.serverList.Update(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
	m.serverList, _ = m.serverList.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	m.serverList, _ = m.serverList.Update(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
	if m.serverList.SelectionCount() != 2 {
		t.Fatalf("selection=%d", m.serverList.SelectionCount())
	}
	m.width, m.height = 200, 40

	del, _ := m.openDeleteConfirm()
	reboot, _ := m.openRebootConfirm("reboot")
	toggle, _ := m.openToggleConfirm("stop/start")
	for _, tc := range []struct {
		name string
		got  Model
		want string
	}{
		{"delete", del, "Delete 2 servers: alpha, beta"},
		{"reboot", reboot, "Reboot 2 servers: alpha, beta"},
		{"mixed toggle", toggle, "Apply stop/start to 2 servers (start:1, stop:1): alpha, beta"},
	} {
		if tc.got.confirm.Body != tc.want {
			t.Errorf("%s body = %q, want %q", tc.name, tc.got.confirm.Body, tc.want)
		}
		if v := tc.got.confirm.View(); strings.Contains(v, `"2 servers"`) {
			t.Errorf("%s view names a server called \"2 servers\":\n%s", tc.name, v)
		}
	}
}
