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
	"github.com/larkly/lazystack/internal/ui/columnpicker"
	"github.com/larkly/lazystack/internal/ui/keypaircreate"
	"github.com/larkly/lazystack/internal/ui/projectpicker"
	"github.com/larkly/lazystack/internal/ui/servercreate"
	"github.com/larkly/lazystack/internal/ui/serverlist"
	"github.com/larkly/lazystack/internal/ui/volumecreate"
)

// press builds a key press for a single printable rune or a named key.
func press(s string) tea.KeyPressMsg {
	switch s {
	case "ctrl+c":
		return tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
	case "esc":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})
	case "enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg(tea.Key{Code: r, Text: s})
}

// quickMessages executes a command tree and collects the messages that are
// produced promptly. Timer-based commands (cursor blinks, refresh ticks)
// are abandoned after a short wait so tests never sleep for real intervals.
func quickMessages(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(50 * time.Millisecond):
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, quickMessages(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func quits(cmd tea.Cmd) bool {
	for _, msg := range quickMessages(cmd) {
		if _, ok := msg.(tea.QuitMsg); ok {
			return true
		}
	}
	return false
}

func typeKeys(t *testing.T, m Model, keys ...string) (Model, bool) {
	t.Helper()
	quit := false
	for _, k := range keys {
		next, cmd := m.Update(press(k))
		m = next.(Model)
		if quits(cmd) {
			quit = true
		}
	}
	return m, quit
}

func TestServerListFilterKeepsQAsText(t *testing.T) {
	m := initTestModel()
	m.view = viewServerList
	m.serverList = serverlist.New(nil, nil, time.Hour)
	m.serverList.SetSize(m.width, m.height)
	m.serverList.SetConfig(m.configView.Cfg())
	m, _ = typeKeys(t, m, "/")
	if !m.serverList.IsFiltering() {
		t.Fatal("setup: filter mode not active")
	}
	m, quit := typeKeys(t, m, "q", "e", "m", "u")
	if quit {
		t.Fatal("typing q in the server filter quit the app")
	}
	if !m.serverList.IsFiltering() || !strings.Contains(m.serverList.View(), "qemu") {
		t.Fatal("filter text did not receive qemu")
	}
	if _, quit = typeKeys(t, m, "ctrl+c"); !quit {
		t.Fatal("ctrl+c must still quit while filtering")
	}
	// Naming a saved filter is text entry too.
	m, _ = typeKeys(t, m, "enter", "f")
	if !m.textInputFocused() {
		t.Fatal("saved-filter name input should capture keys")
	}
	if _, quit = typeKeys(t, m, "q", "1", "R"); quit {
		t.Fatal("typing q in the filter name quit the app")
	}
}

func TestCtrlCQuitsFromEveryFocusState(t *testing.T) {
	cases := map[string]func(m *Model){
		"server create": func(m *Model) {
			m.view = viewServerCreate
			m.serverCreate = servercreate.New(nil, nil, nil)
		},
		"volume create": func(m *Model) {
			m.view = viewVolumeCreate
			m.volumeCreate = volumecreate.New(nil)
		},
		"keypair create": func(m *Model) {
			m.view = viewKeypairCreate
			m.keypairCreate = keypaircreate.New(nil)
		},
		"idle paused":    func(m *Model) { m.idlePaused = true },
		"help":           func(m *Model) { m.help.Open("serverlist") },
		"quota":          func(m *Model) { m.quotaView.Visible = true },
		"config":         func(m *Model) { m.configView.Open() },
		"confirm modal":  func(m *Model) { m.activeModal = modalConfirm },
		"error modal":    func(m *Model) { m.activeModal = modalError },
		"cloud picker":   func(m *Model) { m.view = viewCloudPicker },
		"column picker":  func(m *Model) { m.columnPicker = columnpicker.New(nil) },
		"project picker": func(m *Model) { m.projectPicker = projectpicker.New(nil, "") },
		"server filter": func(m *Model) {
			m.serverList = testServerListFiltering("abc")
		},
	}
	for name, activate := range overlayActivators() {
		cases["overlay "+name] = activate
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			m := initTestModel()
			m.view = viewServerList
			m.serverList = serverlist.New(nil, nil, time.Hour)
			setup(&m)
			_, cmd := m.Update(press("ctrl+c"))
			if !quits(cmd) {
				t.Fatal("ctrl+c did not quit")
			}
		})
	}
}

func TestLBAndImageSearchCaptureTypedKeys(t *testing.T) {
	cases := []struct {
		tab  string
		view activeView
	}{
		{"loadbalancers", viewLBView},
		{"images", viewImageView},
	}
	for _, tc := range cases {
		t.Run(tc.tab, func(t *testing.T) {
			m, requests := listFixture(t)
			m.tabs = []TabDef{{Key: "servers"}, {Key: tc.tab}, {Key: "volumes"}, {Key: "keypairs"}}
			m.tabInited = make([]bool, len(m.tabs))
			var cmd tea.Cmd
			m, cmd = m.switchTab(1)
			for _, msg := range commandMessages(cmd) {
				m, _ = m.updateActiveView(msg)
			}
			m, _ = typeKeys(t, m, "/")
			*requests = nil
			typed := strings.Split("q123hlRodwY", "")
			m, quit := typeKeys(t, m, typed...)
			if quit {
				t.Fatal("typing in search quit the app")
			}
			if m.view != tc.view || m.activeTab != 1 {
				t.Fatalf("search keys switched view/tab: view=%v tab=%d", m.view, m.activeTab)
			}
			if m.activeModal != modalNone || m.copyPicker.Active {
				t.Fatal("search keys opened a confirmation or picker")
			}
			if len(*requests) != 0 {
				t.Fatalf("search keys issued API requests: %v", *requests)
			}
			if got := searchFilter(m, tc.view); got != "q123hlRodwY" || !m.textInputFocused() {
				t.Fatalf("query = %q, want the typed text while searching", got)
			}
			// Enter keeps the filter and leaves search mode; the next key is
			// an ordinary shortcut again.
			m, _ = typeKeys(t, m, "enter")
			if m.view != tc.view || m.textInputFocused() {
				t.Fatal("enter should leave search mode on the same view")
			}
			m, _ = typeKeys(t, m, "/")
			m, _ = typeKeys(t, m, "esc")
			if m.view != tc.view || m.textInputFocused() {
				t.Fatal("esc should leave search mode on the same view")
			}
			m, _ = typeKeys(t, m, "/")
			if _, quit := typeKeys(t, m, "ctrl+c"); !quit {
				t.Fatal("ctrl+c must quit while searching")
			}
		})
	}
}

func searchFilter(m Model, view activeView) string {
	child := reflect.ValueOf(m.lbView)
	if view == viewImageView {
		child = reflect.ValueOf(m.imageView)
	}
	return child.FieldByName("searchFilter").String()
}

func TestCreateFormsReceiveUppercaseRAndY(t *testing.T) {
	for _, view := range []activeView{viewServerCreate, viewVolumeCreate, viewKeypairCreate} {
		m := initTestModel()
		m.view = view
		m.serverCreate = servercreate.New(nil, nil, nil)
		m.serverCreate.SetSize(m.width, m.height)
		m.volumeCreate = volumecreate.New(nil)
		m.volumeCreate.SetSize(m.width, m.height)
		m.keypairCreate = keypaircreate.New(nil)
		m.keypairCreate.SetSize(m.width, m.height)
		m, _ = typeKeys(t, m, "x", "R", "Y", "z")
		if m.copyPicker.Active {
			t.Errorf("view %d: Y opened the copy picker from a create form", view)
		}
		if !strings.Contains(m.viewContent(), "xRYz") {
			t.Errorf("view %d: typed xRYz did not reach the name input", view)
		}
	}
	// Outside text input, R still refreshes.
	m := initTestModel()
	m.view = viewServerList
	m.serverList = serverlist.New(nil, nil, time.Hour)
	if _, cmd := m.Update(press("R")); cmd == nil {
		t.Fatal("R on the server list should refresh")
	}
}

func serverListWithServers(t *testing.T) Model {
	t.Helper()
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/servers/detail":
			fmt.Fprint(w, `{"servers":[{"id":"a","name":"alpha","status":"ACTIVE"},{"id":"b","name":"beta","status":"ACTIVE"},{"id":"c","name":"gamma","status":"ACTIVE"}]}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	})
	m.view = viewServerList
	m.serverList = serverlist.New(m.client.Compute, m.client.Image, time.Hour)
	m.serverList.SetConfig(m.configView.Cfg())
	m.serverList.SetSize(m.width, m.height)
	for _, msg := range commandMessages(m.serverList.Init()) {
		m.serverList, _ = m.serverList.Update(msg)
	}
	if m.serverList.SelectedServer() == nil {
		t.Fatal("setup: no servers loaded")
	}
	return m
}

func TestServerListEscClearsSelection(t *testing.T) {
	m := serverListWithServers(t)
	m.returnToView = viewServerDetail
	m.serverDetail = testServerDetailWithServer("srv-1", "srv-1")
	m, _ = typeKeys(t, m, " ", " ")
	if m.serverList.SelectionCount() != 2 || !strings.Contains(m.statusBar.Hint, "esc clear") {
		t.Fatalf("setup: selection=%d hint=%q", m.serverList.SelectionCount(), m.statusBar.Hint)
	}
	m, quit := typeKeys(t, m, "esc")
	if quit || m.view != viewServerList || m.activeModal != modalNone {
		t.Fatalf("esc with a selection navigated or acted: view=%v modal=%v quit=%v", m.view, m.activeModal, quit)
	}
	if m.serverList.SelectionCount() != 0 {
		t.Fatalf("selection = %d after esc, want 0", m.serverList.SelectionCount())
	}
	if strings.Contains(m.statusBar.Hint, "selected") {
		t.Fatalf("hint still advertises a selection: %q", m.statusBar.Hint)
	}
	// With nothing selected, esc keeps its back-navigation meaning.
	m, _ = typeKeys(t, m, "esc")
	if m.view != viewServerDetail {
		t.Fatalf("second esc should return to detail, got view %v", m.view)
	}
}

func TestServerListEscInFilterClearsFilterNotSelection(t *testing.T) {
	m := serverListWithServers(t)
	m, _ = typeKeys(t, m, " ", "/", "a")
	m, _ = typeKeys(t, m, "esc")
	if m.serverList.IsFiltering() {
		t.Fatal("esc should leave filter mode")
	}
	if m.serverList.SelectionCount() != 1 {
		t.Fatalf("filter esc must not clear the selection, got %d", m.serverList.SelectionCount())
	}
}

func TestCreateFormsKeepQAsInput(t *testing.T) {
	for _, view := range []activeView{viewServerCreate, viewVolumeCreate, viewKeypairCreate} {
		m := initTestModel()
		m.view = view
		m.serverCreate = servercreate.New(nil, nil, nil)
		m.volumeCreate = volumecreate.New(nil)
		m.keypairCreate = keypaircreate.New(nil)
		if _, quit := typeKeys(t, m, "q"); quit {
			t.Errorf("view %d: plain q quit from a create form", view)
		}
	}
	_ = shared.Keys
}
