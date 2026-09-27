package app

import (
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/ui/modal"
)

// resizeFailFixture serves flavors, deletes any server and fails every
// resize with a conflict.
func resizeFailFixture(t *testing.T) Model {
	t.Helper()
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/flavors/detail"):
			w.Write([]byte(`{"flavors":[{"id":"f-large","name":"m1.large","vcpus":4,"ram":8192,"disk":80}]}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"conflictingRequest":{"code":409,"message":"resize refused"}}`))
		}
	})
	return m
}

// submitResize opens the resize modal on srv-b's detail view, submits it
// and returns the tracked resize result, not yet delivered.
func submitResize(t *testing.T, m Model) (Model, tea.Msg) {
	t.Helper()
	m.view = viewServerDetail
	m.serverDetail = detailFor("srv-b", "ACTIVE")
	m = openResizeModal(t, m)
	res, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = res.(Model)
	for _, msg := range nonSpinner(cmd) {
		if _, ok := msg.(actionResultMsg); ok {
			return m, msg
		}
	}
	t.Fatal("resize was not submitted")
	return m, nil
}

// assertResizeFailureShown delivers the resize result and checks that its
// failure reaches the user instead of being dropped.
func assertResizeFailureShown(t *testing.T, m Model, result tea.Msg) {
	t.Helper()
	if !m.serverResize.Active {
		t.Fatal("an unrelated result closed the resize modal while its resize was in flight")
	}
	res, _ := m.Update(result)
	m = res.(Model)
	if !m.serverResize.Active || !strings.Contains(m.serverResize.View(), "resize refused") {
		t.Fatalf("resize failure was dropped:\n%s", m.serverResize.View())
	}
}

// Deleting server A must not close the resize modal of server B: the modal
// would then drop B's result and a failed resize would show no error.
func TestServerDeletedKeepsUnrelatedResizeModal(t *testing.T) {
	m := resizeFailFixture(t)
	res, del := m.Update(confirmed("delete", "srv-a"))
	m = res.(Model)
	if del == nil {
		t.Fatal("delete not scheduled")
	}
	m, result := submitResize(t, m)
	m, _ = deliver(t, m, del)
	assertResizeFailureShown(t, m, result)
}

// A bulk server action finishing must not close the resize modal either.
func TestBulkServerResultKeepsUnrelatedResizeModal(t *testing.T) {
	m := resizeFailFixture(t)
	m, result := submitResize(t, m)
	res, _ := m.Update(bulkResultMsg{
		resource:  "server",
		label:     "reboot",
		noun:      "servers",
		succeeded: []modal.ServerRef{{ID: "srv-a", Name: "srv-a"}},
	})
	m = res.(Model)
	assertResizeFailureShown(t, m, result)
}
