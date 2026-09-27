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
)

func newModel(t *testing.T, meta map[string]string) (Model, *metaRecorder) {
	t.Helper()
	rec := &metaRecorder{}
	client, cleanup := testutil.FakeServiceClient(rec)
	t.Cleanup(cleanup)
	m := New(client, "srv-1", "web", meta)
	m.SetSize(100, 40)
	return m, rec
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
		{"delete", func(m Model) Model { return m }, []tea.KeyPressMsg{{Code: 'd', Text: "d"}}},
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
	m, cmd := press(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if !m.submitting {
		t.Fatal("delete did not lock")
	}
	m, _ = m.Update(cmd())
	if m.submitting || !m.Active || !strings.Contains(m.View(), "Delete metadata") {
		t.Fatalf("error not surfaced: submitting=%v active=%v\n%s", m.submitting, m.Active, m.View())
	}
	rec.failDel = false
	m, cmd = press(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
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
		if m.Active || done == nil {
			t.Fatal("successful rename did not close")
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
	_, cmd := press(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
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
	if _, cmd := press(empty, tea.KeyPressMsg{Code: 'd', Text: "d"}); cmd != nil {
		t.Fatal("delete on empty map produced a command")
	}

	// Removing the selected final key reclamps the cursor.
	m2, _ := newModel(t, map[string]string{"a": "1", "b": "2"})
	m2, _ = press(m2, keyDown)
	if m2.cursor != 1 {
		t.Fatalf("cursor=%d", m2.cursor)
	}
	m2, cmd = press(m2, tea.KeyPressMsg{Code: 'd', Text: "d"})
	m2, _ = m2.Update(cmd())
	if m2.cursor != 0 {
		t.Fatalf("cursor after deleting last key=%d", m2.cursor)
	}
}

func TestDeleteReportsDeletion(t *testing.T) {
	m, _ := newModel(t, map[string]string{"role": "db"})
	m, cmd := press(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
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
