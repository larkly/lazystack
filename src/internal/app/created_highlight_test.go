package app

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/keypairlist"
	"github.com/larkly/lazystack/internal/ui/serverlist"
	"github.com/larkly/lazystack/internal/ui/volumelist"
)

// After a full-screen create form succeeds, the app shows the create
// result and the list lands its cursor on the new resource.
func TestCreateFormResultHighlightsNewResource(t *testing.T) {
	cases := []struct {
		form     activeView
		tab      int
		target   string
		id       string
		selected func(m Model) string
	}{
		{viewServerCreate, 0, "serverlist", "b", func(m Model) string {
			if s := m.serverList.SelectedServer(); s != nil {
				return s.ID
			}
			return ""
		}},
		{viewVolumeCreate, 1, "volumelist", "b", func(m Model) string {
			if v := m.volumeList.SelectedVolume(); v != nil {
				return v.ID
			}
			return ""
		}},
		{viewKeypairCreate, 2, "keypairlist", "b", func(m Model) string {
			if kp := m.keypairList.SelectedKeyPair(); kp != nil {
				return kp.Name
			}
			return ""
		}},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/servers/detail":
					fmt.Fprint(w, `{"servers":[{"id":"a","name":"a","status":"ACTIVE"},{"id":"b","name":"b","status":"ACTIVE"}]}`)
				case "/volumes/detail":
					fmt.Fprint(w, `{"volumes":[{"id":"a","name":"a","status":"available"},{"id":"b","name":"b","status":"available"}]}`)
				case "/os-keypairs":
					fmt.Fprint(w, `{"keypairs":[{"keypair":{"name":"a"}},{"keypair":{"name":"b"}}]}`)
				default:
					fmt.Fprint(w, `{}`)
				}
			})
			m.serverList = serverlist.New(m.client.Compute, m.client.Image, time.Hour)
			m.volumeList = volumelist.New(m.client.BlockStorage, m.client.Compute, time.Hour)
			m.keypairList = keypairlist.New(m.client.Compute, time.Hour)
			next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			m = next.(Model)
			// Each form opens from its own tab, which receives the list
			// results.
			m.tabs = []TabDef{{Name: "Servers", Key: "servers"}, {Name: "Volumes", Key: "volumes"}, {Name: "Key Pairs", Key: "keypairs"}}
			m.tabInited = []bool{true, true, true}
			m.activeTab = tc.tab
			m.view = tc.form
			update := func(msg tea.Msg) tea.Cmd {
				next, cmd := m.Update(msg)
				m = next.(Model)
				return cmd
			}
			loadViewData(update(shared.ViewChangeMsg{View: tc.target, Highlight: tc.id}), update)
			loadViewData(update(shared.ResourceActionMsg{Action: "Created", Name: "new"}), update)
			if got := tc.selected(m); got != tc.id {
				t.Errorf("cursor on %q, want %q", got, tc.id)
			}
			if !strings.Contains(m.statusBar.StickyHint, "Created new") {
				t.Errorf("status bar = %q, want the create result", m.statusBar.StickyHint)
			}
		})
	}
}
