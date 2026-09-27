package app

import (
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
