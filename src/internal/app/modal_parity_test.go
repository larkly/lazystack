package app

import (
	"net/http"
	"reflect"
	"sort"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/cloneprogress"
	"github.com/larkly/lazystack/internal/ui/servermetadata"
)

// overlayActivators lists every Model field that is an overlay with an
// Active flag, keyed by field name. TestOverlayActivatorsCoverEveryOverlay
// fails when a new overlay field is added without being listed here, which
// in turn forces the render/key/resize parity checks below to cover it.
func overlayActivators() map[string]func(m *Model) {
	return map[string]func(m *Model){
		"cloneProgress":    func(m *Model) { m.cloneProgress.Active = true },
		"serverRename":     func(m *Model) { m.serverRename.Active = true },
		"serverRebuild":    func(m *Model) { m.serverRebuild.Active = true },
		"serverSnapshot":   func(m *Model) { m.serverSnapshot.Active = true },
		"serverResize":     func(m *Model) { m.serverResize.Active = true },
		"serverAdminAct":   func(m *Model) { m.serverAdminAct.Active = true },
		"serverMetadata":   func(m *Model) { m.serverMetadata.Active = true },
		"sshPrompt":        func(m *Model) { m.sshPrompt.Active = true },
		"copyPicker":       func(m *Model) { m.copyPicker.Active = true },
		"consoleURL":       func(m *Model) { m.consoleURL.Active = true },
		"vmPassword":       func(m *Model) { m.vmPassword.Active = true },
		"fipPicker":        func(m *Model) { m.fipPicker.Active = true },
		"serverPicker":     func(m *Model) { m.serverPicker.Active = true },
		"volumePicker":     func(m *Model) { m.volumePicker.Active = true },
		"routerCreate":     func(m *Model) { m.routerCreate.Active = true },
		"subnetPicker":     func(m *Model) { m.subnetPicker.Active = true },
		"networkCreate":    func(m *Model) { m.networkCreate.Active = true },
		"subnetCreate":     func(m *Model) { m.subnetCreate.Active = true },
		"subnetEdit":       func(m *Model) { m.subnetEdit.Active = true },
		"portCreate":       func(m *Model) { m.portCreate.Active = true },
		"portEdit":         func(m *Model) { m.portEdit.Active = true },
		"sgCreate":         func(m *Model) { m.sgCreate.Active = true },
		"sgRuleCreate":     func(m *Model) { m.sgRuleCreate.Active = true },
		"imageEdit":        func(m *Model) { m.imageEdit.Active = true },
		"imageCreate":      func(m *Model) { m.imageCreate.Active = true },
		"imageDownload":    func(m *Model) { m.imageDownload.Active = true },
		"lbCreate":         func(m *Model) { m.lbCreate.Active = true },
		"lbListenerCreate": func(m *Model) { m.lbListenerCreate.Active = true },
		"lbPoolCreate":     func(m *Model) { m.lbPoolCreate.Active = true },
		"lbMemberCreate":   func(m *Model) { m.lbMemberCreate.Active = true },
		"lbMonitorCreate":  func(m *Model) { m.lbMonitorCreate.Active = true },
		"projectPicker":    func(m *Model) { m.projectPicker.Active = true },
		"columnPicker":     func(m *Model) { m.columnPicker.Active = true },
	}
}

// overlayFieldNames returns the names of Model fields whose type is a struct
// with an exported "Active bool" field.
func overlayFieldNames() []string {
	var names []string
	rt := reflect.TypeOf(Model{})
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Type.Kind() != reflect.Struct {
			continue
		}
		if a, ok := f.Type.FieldByName("Active"); ok && a.Type.Kind() == reflect.Bool {
			names = append(names, f.Name)
		}
	}
	sort.Strings(names)
	return names
}

// Overlays that intentionally keep a fixed size or are sized elsewhere.
var overlaysWithoutSetSize = map[string]bool{}

func TestOverlayActivatorsCoverEveryOverlay(t *testing.T) {
	acts := overlayActivators()
	for _, name := range overlayFieldNames() {
		if _, ok := acts[name]; !ok {
			t.Errorf("overlay field %q is missing from overlayActivators", name)
		}
	}
	if len(acts) != len(overlayFieldNames()) {
		t.Errorf("overlayActivators lists %d overlays, model has %d", len(acts), len(overlayFieldNames()))
	}
}

func TestEveryOverlayRendersAndCapturesKeys(t *testing.T) {
	for name, activate := range overlayActivators() {
		t.Run(name, func(t *testing.T) {
			m := initTestModel()
			m.view = viewServerList
			activate(&m)
			if o, ok := m.topOverlay(); !ok || o.View() == "" {
				t.Error("the active overlay is not rendered")
			}
			if ok, _ := m.updateAnyModal(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"})); !ok {
				t.Error("updateAnyModal does not route keys to the active overlay")
			}
		})
	}
}

func TestResizeReachesEveryOverlay(t *testing.T) {
	m := initTestModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 171, Height: 61})
	m = next.(Model)
	rv := reflect.ValueOf(m)
	for _, name := range overlayFieldNames() {
		if overlaysWithoutSetSize[name] {
			continue
		}
		child := rv.FieldByName(name)
		w := child.FieldByName("width")
		h := child.FieldByName("height")
		if !w.IsValid() || !h.IsValid() {
			t.Errorf("%s has no width/height fields; add an explicit exception", name)
			continue
		}
		if w.Int() != 171 || h.Int() != 61 {
			t.Errorf("%s size = %dx%d, want 171x61", name, w.Int(), h.Int())
		}
	}
}

// Async results of an overlay must reach it while it is open; the metadata
// editor used to stay stuck because its completion message was never routed
// back to it.
func TestServerMetadataReceivesBackgroundResults(t *testing.T) {
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/servers/srv/metadata/k" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	m.view = viewServerDetail
	m.serverMetadata = servermetadata.New(m.client.Compute, "srv", "web", map[string]string{"k": "v"})
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	m = next.(Model)
	var sawAction bool
	for _, msg := range commandMessages(cmd) {
		next, cmd = m.Update(msg)
		m = next.(Model)
		for _, out := range commandMessages(cmd) {
			if _, ok := out.(shared.ServerActionMsg); ok {
				sawAction = true
			}
		}
	}
	if m.serverMetadata.Active || !sawAction {
		t.Fatalf("metadata result not delivered: active=%v action=%v", m.serverMetadata.Active, sawAction)
	}
}

// overlayPrecedence is the order in which overlays win when several are
// active at once. It must match what the screen shows, keys reach and
// background messages reach.
var overlayPrecedence = []string{
	"confirm", "errModal", "cloneProgress",
	"serverRename", "serverRebuild", "serverSnapshot", "serverResize",
	"serverAdminAct", "serverMetadata", "sshPrompt", "copyPicker",
	"consoleURL", "vmPassword", "fipPicker", "serverPicker", "volumePicker",
	"routerCreate", "subnetPicker", "networkCreate", "subnetCreate",
	"subnetEdit", "portCreate", "portEdit", "sgCreate", "sgRuleCreate",
	"imageEdit", "imageCreate", "imageDownload", "lbCreate",
	"lbListenerCreate", "lbPoolCreate", "lbMemberCreate", "lbMonitorCreate",
	"projectPicker", "columnPicker",
}

// Confirm and error modals take keys through updateModal and do no async
// work; image upload and download take background messages through their
// own always-on routes so backgrounded transfers keep flowing.
var (
	overlaysWithOwnKeyRoute        = map[string]bool{"confirm": true, "errModal": true}
	overlaysWithOwnBackgroundRoute = map[string]bool{
		"confirm": true, "errModal": true, "imageCreate": true, "imageDownload": true,
	}
)

func TestOverlayListMatchesPrecedence(t *testing.T) {
	m := initTestModel()
	var got []string
	for _, o := range m.overlays() {
		got = append(got, o.name)
		if o.update == nil || o.setSize == nil || o.overlay == nil {
			t.Errorf("%s: overlay entry is incomplete", o.name)
		}
	}
	if !reflect.DeepEqual(got, overlayPrecedence) {
		t.Fatalf("overlay order\n got %v\nwant %v", got, overlayPrecedence)
	}
	acts := overlayActivators()
	for name := range acts {
		if !contains(got, name) {
			t.Errorf("overlay %q is missing from the overlay list", name)
		}
	}
	if len(got) != len(acts)+len(overlaysWithOwnKeyRoute) {
		t.Errorf("overlay list has %d entries, want %d", len(got), len(acts)+len(overlaysWithOwnKeyRoute))
	}
}

// Every overlay must be reachable by every path: the screen, the status-bar
// hints, key presses and background messages. The background path used to
// skip sshPrompt, serverAdminAct and columnPicker.
func TestEveryOverlayIsReachedByEveryPath(t *testing.T) {
	for name, activate := range overlayActivators() {
		t.Run(name, func(t *testing.T) {
			m := initTestModel()
			activate(&m)
			if o, ok := m.topOverlay(); !ok || o.name != name {
				t.Errorf("render path picks %q (ok=%v)", o.name, ok)
			} else if o.Hints() == "" {
				t.Error("overlay offers no status-bar hints")
			}
			if o, ok := m.keyOverlay(); overlaysWithOwnKeyRoute[name] == ok || (ok && o.name != name) {
				t.Errorf("key path picks %q (ok=%v)", o.name, ok)
			}
			o, ok := m.backgroundOverlay()
			if overlaysWithOwnBackgroundRoute[name] {
				if ok {
					t.Errorf("background path picks %q; it has its own route", o.name)
				}
			} else if !ok || o.name != name {
				t.Errorf("background path picks %q (ok=%v)", o.name, ok)
			}
		})
	}
	for _, kind := range []modalType{modalConfirm, modalError} {
		m := initTestModel()
		m.activeModal = kind
		o, ok := m.topOverlay()
		if !ok || !overlaysWithOwnKeyRoute[o.name] {
			t.Errorf("modal %d: render path picks %q (ok=%v)", kind, o.name, ok)
		}
	}
}

// A dismissed clone that is still running keeps its spinner ticking when no
// overlay is open, but must not steal messages from an open overlay.
func TestRunningCloneTakesBackgroundOnlyWhenNothingElseIsOpen(t *testing.T) {
	m := initTestModel()
	m.cloneProgress = cloneprogress.New(nil, nil, "srv", "web", nil)
	m.cloneProgress.Active = false // dismissed, still running
	if !m.cloneTakesBackground() {
		t.Fatal("a dismissed running clone gets no background messages")
	}
	m.serverRename.Active = true
	if m.cloneTakesBackground() {
		t.Fatal("a dismissed clone takes background messages from an open overlay")
	}
	if o, ok := m.backgroundOverlay(); !ok || o.name != "serverRename" {
		t.Fatalf("background path picks %q (ok=%v)", o.name, ok)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
