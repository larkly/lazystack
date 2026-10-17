package servermetadata

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

// metaRecorder is a fake Nova metadata endpoint that records every request
// and can be told to fail specific methods.
type metaRecorder struct {
	mu       sync.Mutex
	requests []string
	failPut  bool
	failDel  bool
}

func (r *metaRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.requests = append(r.requests, req.Method+" "+req.URL.Path)
	failPut, failDel := r.failPut, r.failDel
	r.mu.Unlock()
	switch req.Method {
	case http.MethodPut:
		if failPut {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"meta":{}}`))
	case http.MethodDelete:
		if failDel {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (r *metaRecorder) got() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requests...)
}

func press(m Model, k tea.KeyPressMsg) (Model, tea.Cmd) {
	return m.Update(k)
}

func runes(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

var (
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
	keyTab   = tea.KeyPressMsg{Code: tea.KeyTab}
	keyD     = tea.KeyPressMsg{Code: 'd', Text: "d"}
	keyY     = tea.KeyPressMsg{Code: 'y', Text: "y"}
	keyN     = tea.KeyPressMsg{Code: 'n', Text: "n"}
)

// deleteSelected presses d and confirms with y.
func deleteSelected(m Model) (Model, tea.Cmd) {
	m, _ = press(m, keyD)
	return press(m, keyY)
}

func newModel(t *testing.T, meta map[string]string) (Model, *metaRecorder) {
	t.Helper()
	rec := &metaRecorder{}
	client, cleanup := testutil.FakeServiceClient(rec)
	t.Cleanup(cleanup)
	m := New(client, "srv-1", "web", meta)
	m.SetSize(100, 40)
	return m, rec
}

// j/k must be typed into the key/value inputs, not switch fields; only
// tab/shift+tab and the arrows navigate.
func TestFormTypesJKAndNavigatesWithTabAndArrows(t *testing.T) {
	for _, mode := range []string{"a", "e"} {
		m, _ := newModel(t, map[string]string{"role": "db"})
		m = runes(m, mode)
		m.keyInput.SetValue("")
		m.valueInput.SetValue("")
		m = runes(m, "project_id_jk")
		if !m.keyInput.Focused() || m.keyInput.Value() != "project_id_jk" {
			t.Fatalf("%s: key=%q focused=%v", mode, m.keyInput.Value(), m.keyInput.Focused())
		}
		for _, k := range []tea.KeyPressMsg{keyTab, {Code: tea.KeyTab, Mod: tea.ModShift}, keyDown, {Code: tea.KeyUp}} {
			wasKey := m.keyInput.Focused()
			m, _ = press(m, k)
			if m.keyInput.Focused() == wasKey || m.valueInput.Focused() != wasKey {
				t.Fatalf("%s: %q did not switch fields", mode, k.String())
			}
		}
		m, _ = press(m, keyTab)
		m = runes(m, "jk")
		if !m.valueInput.Focused() || m.valueInput.Value() != "jk" {
			t.Fatalf("%s: value=%q focused=%v", mode, m.valueInput.Value(), m.valueInput.Focused())
		}
	}
}

func TestBlankKeyIsRejectedWithoutRequests(t *testing.T) {
	for _, mode := range []string{"a", "e"} {
		m, rec := newModel(t, map[string]string{"role": "db"})
		m = runes(m, mode)
		m.keyInput.SetValue("   ")
		var cmd tea.Cmd
		m, cmd = press(m, keyEnter)
		if cmd != nil {
			t.Fatalf("%s: blank key produced a command", mode)
		}
		if m.submitting {
			t.Fatalf("%s: blank key left the editor submitting", mode)
		}
		if !strings.Contains(m.View(), "Key is required") {
			t.Fatalf("%s: missing validation error:\n%s", mode, m.View())
		}
		if got := rec.got(); len(got) != 0 {
			t.Fatalf("%s: requests=%v", mode, got)
		}
	}
}

func TestSubmittingLocksEditor(t *testing.T) {
	cases := []struct {
		name  string
		setup func(Model) Model
		keys  []tea.KeyPressMsg
	}{
		{"add", func(m Model) Model {
			m = runes(m, "a")
			m = runes(m, "owner")
			m, _ = press(m, keyTab)
			return runes(m, "ops")
		}, []tea.KeyPressMsg{keyEnter}},
		{"edit", func(m Model) Model {
			m = runes(m, "e")
			m, _ = press(m, keyTab)
			return runes(m, "x")
		}, []tea.KeyPressMsg{keyEnter}},
		{"delete", func(m Model) Model {
			m, _ = press(m, keyD)
			return m
		}, []tea.KeyPressMsg{keyY}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, rec := newModel(t, map[string]string{"role": "db"})
			m = tc.setup(m)
			var cmd tea.Cmd
			m, cmd = press(m, tc.keys[0])
			if cmd == nil || !m.submitting {
				t.Fatalf("submit: cmd=%v submitting=%v", cmd != nil, m.submitting)
			}
			// Repeated submit and Escape must be ignored while in flight.
			for _, k := range []tea.KeyPressMsg{tc.keys[0], keyEnter, {Code: 'd', Text: "d"}, keyEsc} {
				var again tea.Cmd
				m, again = press(m, k)
				if again != nil {
					t.Fatalf("key %q while submitting produced a command", k.String())
				}
			}
			if !m.Active || !m.submitting {
				t.Fatalf("in-flight editor abandoned: active=%v submitting=%v", m.Active, m.submitting)
			}
			msg := cmd()
			if n := len(rec.got()); n != 1 {
				t.Fatalf("requests=%v", rec.got())
			}
			m, _ = m.Update(msg)
			if m.submitting {
				t.Fatal("completion did not clear the submit lock")
			}
		})
	}
}

func TestSubmitErrorUnlocksEditor(t *testing.T) {
	m, rec := newModel(t, map[string]string{"role": "db"})
	rec.failDel = true
	m, cmd := deleteSelected(m)
	if !m.submitting {
		t.Fatal("delete did not lock")
	}
	m, _ = m.Update(cmd())
	if m.submitting || !m.Active || !strings.Contains(m.View(), "Delete metadata") {
		t.Fatalf("error not surfaced: submitting=%v active=%v\n%s", m.submitting, m.Active, m.View())
	}
	rec.failDel = false
	m, cmd = deleteSelected(m)
	if cmd == nil || !m.submitting {
		t.Fatal("retry after error not accepted")
	}
}

func TestRenameWritesNewKeyBeforeRemovingOld(t *testing.T) {
	edit := func(t *testing.T) (Model, *metaRecorder, tea.Cmd) {
		m, rec := newModel(t, map[string]string{"old": "v1"})
		m = runes(m, "e")
		m.keyInput.SetValue("new")
		var cmd tea.Cmd
		m, cmd = press(m, keyEnter)
		if cmd == nil {
			t.Fatal("rename produced no command")
		}
		return m, rec, cmd
	}

	t.Run("success", func(t *testing.T) {
		m, rec, cmd := edit(t)
		msg := cmd()
		want := []string{"PUT /servers/srv-1/metadata/new", "DELETE /servers/srv-1/metadata/old"}
		if got := rec.got(); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("requests=%v want %v", got, want)
		}
		m, done := m.Update(msg)
		if !m.Active || done == nil {
			t.Fatal("successful rename closed the editor or reported nothing")
		}
		if _, ok := m.metadata["old"]; ok || m.metadata["new"] != "v1" {
			t.Fatalf("local view after rename=%v", m.metadata)
		}
	})

	t.Run("put fails", func(t *testing.T) {
		m, rec, cmd := edit(t)
		rec.failPut = true
		m, _ = m.Update(cmd())
		if got := rec.got(); len(got) != 1 || got[0] != "PUT /servers/srv-1/metadata/new" {
			t.Fatalf("failed write must not delete the original: requests=%v", got)
		}
		if !m.Active || m.metadata["old"] != "v1" || !strings.Contains(m.err, "Update metadata") {
			t.Fatalf("original not retained: active=%v meta=%v err=%q", m.Active, m.metadata, m.err)
		}
	})

	t.Run("delete fails", func(t *testing.T) {
		m, rec, cmd := edit(t)
		rec.failDel = true
		m, _ = m.Update(cmd())
		if got := rec.got(); len(got) != 2 {
			t.Fatalf("requests=%v", got)
		}
		if !m.Active || m.submitting {
			t.Fatalf("partial rename must stay open: active=%v submitting=%v", m.Active, m.submitting)
		}
		if !strings.Contains(m.err, `"new"`) || !strings.Contains(m.err, `"old"`) {
			t.Fatalf("partial failure must name both keys: %q", m.err)
		}
		if m.metadata["old"] != "v1" || m.metadata["new"] != "v1" {
			t.Fatalf("local view must reflect both keys: %v", m.metadata)
		}
		if m.mode != "" {
			t.Fatalf("partial rename should return to the list, mode=%q", m.mode)
		}
	})

	t.Run("same key", func(t *testing.T) {
		m, rec := newModel(t, map[string]string{"old": "v1"})
		m = runes(m, "e")
		m.valueInput.SetValue("v2")
		_, cmd := press(m, keyEnter)
		cmd()
		if got := rec.got(); len(got) != 1 || got[0] != "PUT /servers/srv-1/metadata/old" {
			t.Fatalf("same-key update requests=%v", got)
		}
	})
}

func TestCursorClampsToLastKey(t *testing.T) {
	m, rec := newModel(t, map[string]string{"only": "v"})
	for i := 0; i < 3; i++ {
		m, _ = press(m, keyDown)
	}
	if m.cursor != 0 {
		t.Fatalf("cursor=%d", m.cursor)
	}
	m = runes(m, "e")
	if m.mode != "edit" || m.editKey != "only" {
		t.Fatalf("edit not actionable: mode=%q key=%q", m.mode, m.editKey)
	}
	m, _ = press(m, keyEsc)
	_, cmd := deleteSelected(m)
	if cmd == nil {
		t.Fatal("delete not actionable")
	}
	cmd()
	if got := rec.got(); len(got) != 1 || got[0] != "DELETE /servers/srv-1/metadata/only" {
		t.Fatalf("requests=%v", got)
	}

	empty, _ := newModel(t, nil)
	for i := 0; i < 3; i++ {
		empty, _ = press(empty, keyDown)
		empty, _ = press(empty, tea.KeyPressMsg{Code: tea.KeyUp})
		empty, _ = press(empty, keyDown)
	}
	if empty.cursor != 0 {
		t.Fatalf("empty cursor=%d", empty.cursor)
	}
	if empty, cmd := deleteSelected(empty); cmd != nil || empty.mode != "" {
		t.Fatalf("delete on empty map: cmd=%v mode=%q", cmd != nil, empty.mode)
	}

	// Removing the selected final key reclamps the cursor.
	m2, _ := newModel(t, map[string]string{"a": "1", "b": "2"})
	m2, _ = press(m2, keyDown)
	if m2.cursor != 1 {
		t.Fatalf("cursor=%d", m2.cursor)
	}
	m2, cmd = deleteSelected(m2)
	m2, _ = m2.Update(cmd())
	if m2.cursor != 0 {
		t.Fatalf("cursor after deleting last key=%d", m2.cursor)
	}
}

func TestDeleteReportsDeletion(t *testing.T) {
	m, _ := newModel(t, map[string]string{"role": "db"})
	m, cmd := deleteSelected(m)
	_, done := m.Update(cmd())
	if done == nil {
		t.Fatal("no completion message")
	}
	got := done().(shared.ServerActionMsg)
	if got.Action != "Deleted metadata" || got.Name != "web" {
		t.Fatalf("delete action=%+v", got)
	}

	m, _ = newModel(t, map[string]string{"role": "db"})
	m = runes(m, "a")
	m.keyInput.SetValue("owner")
	m, cmd = press(m, keyEnter)
	_, done = m.Update(cmd())
	if got := done().(shared.ServerActionMsg); got.Action != "Updated metadata" {
		t.Fatalf("add action=%+v", got)
	}
}

func TestRenderSelectedRowKeepsBaseStyles(t *testing.T) {
	m, _ := newModel(t, map[string]string{"a": "1", "b": "2"})
	out := m.renderMetadataList()
	if !strings.Contains(out, "▸ ") {
		t.Fatalf("selected row not marked:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("lines=%q", lines)
	}
	sel := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(28).
		Foreground(shared.ColorHighlight).Render("a")
	if !strings.Contains(lines[0], sel) {
		t.Fatalf("selected key style changed: %q", lines[0])
	}
	unsel := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(28).Render("b")
	if !strings.Contains(lines[1], unsel) {
		t.Fatalf("unselected key style changed: %q", lines[1])
	}
}

// d names the key and the server and waits for y; n and esc keep the key.
func TestDeleteAsksForConfirmation(t *testing.T) {
	m, rec := newModel(t, map[string]string{"role": "db", "zone": "a"})
	m, _ = press(m, keyDown)
	for _, cancel := range []tea.KeyPressMsg{keyN, keyEsc} {
		var cmd tea.Cmd
		m, cmd = press(m, keyD)
		if cmd != nil || m.submitting {
			t.Fatal("d deleted without confirmation")
		}
		if v := m.View(); !strings.Contains(v, `Delete metadata key "zone" from web?`) {
			t.Fatalf("prompt does not name the key and server:\n%s", v)
		}
		if !strings.Contains(m.Hints(), "y delete") {
			t.Fatalf("hints=%q", m.Hints())
		}
		m, cmd = press(m, cancel)
		if cmd != nil || !m.Active || m.mode != "" {
			t.Fatalf("%s: active=%v mode=%q", cancel.String(), m.Active, m.mode)
		}
	}
	if got := rec.got(); len(got) != 0 {
		t.Fatalf("requests after cancel=%v", got)
	}
	m, cmd := deleteSelected(m)
	if cmd == nil || !m.submitting {
		t.Fatal("y did not delete")
	}
	cmd()
	if got := rec.got(); len(got) != 1 || got[0] != "DELETE /servers/srv-1/metadata/zone" {
		t.Fatalf("requests=%v", got)
	}
}

// The editor stays open after each change, mirrors it locally and says it
// was saved; Esc closes it.
func TestEditorStaysOpenAfterEachOperation(t *testing.T) {
	saved := shared.Glyph("✓", "OK") + " saved"
	m, rec := newModel(t, map[string]string{"role": "db"})
	finish := func(m Model, cmd tea.Cmd) Model {
		t.Helper()
		if cmd == nil {
			t.Fatal("no request command")
		}
		m, done := m.Update(cmd())
		if done == nil {
			t.Fatal("no completion message for the app")
		}
		if got, ok := done().(shared.ServerActionMsg); !ok || got.Name != "web" {
			t.Fatalf("completion=%#v", got)
		}
		if !m.Active || m.submitting || m.mode != "" {
			t.Fatalf("after save: active=%v submitting=%v mode=%q", m.Active, m.submitting, m.mode)
		}
		if !strings.Contains(m.View(), saved) {
			t.Fatalf("no saved status:\n%s", m.View())
		}
		return m
	}

	// Add.
	m = runes(m, "a")
	m = runes(m, "owner")
	m, _ = press(m, keyTab)
	m = runes(m, "ops")
	m, cmd := press(m, keyEnter)
	m = finish(m, cmd)
	if m.metadata["owner"] != "ops" || m.metadata["role"] != "db" {
		t.Fatalf("after add=%v", m.metadata)
	}
	if keys := m.sortedKeys(); keys[m.cursor] != "owner" {
		t.Fatalf("cursor on %q, want the saved key", keys[m.cursor])
	}

	// Any key clears the status.
	m, _ = press(m, keyDown)
	if strings.Contains(m.View(), saved) {
		t.Fatal("saved status outlived the next key")
	}

	// Edit with rename: role -> tier.
	m = runes(m, "e")
	m.keyInput.SetValue("tier")
	m.valueInput.SetValue("web")
	m, cmd = press(m, keyEnter)
	m = finish(m, cmd)
	if _, ok := m.metadata["role"]; ok || m.metadata["tier"] != "web" {
		t.Fatalf("after rename=%v", m.metadata)
	}

	// Delete.
	m, cmd = deleteSelected(m)
	m = finish(m, cmd)
	if _, ok := m.metadata["tier"]; ok || len(m.metadata) != 1 {
		t.Fatalf("after delete=%v", m.metadata)
	}

	if n := len(rec.got()); n != 4 {
		t.Fatalf("requests=%v", rec.got())
	}
	m, _ = press(m, keyEsc)
	if m.Active {
		t.Fatal("esc did not close the editor")
	}
}
