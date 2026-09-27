package app

import (
	"net/http"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// resizeModalFixture serves flavors and records server actions.
func resizeModalFixture(t *testing.T) (Model, *requestLog) {
	t.Helper()
	log := &requestLog{}
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/flavors/detail") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"flavors":[{"id":"f-large","name":"m1.large","vcpus":4,"ram":8192,"disk":80}]}`))
			return
		}
		log.add(r.Method + " " + r.URL.Path)
		w.WriteHeader(http.StatusAccepted)
	})
	m.view = viewServerDetail
	m.serverDetail = detailFor("srv-a", "ACTIVE")
	return m, log
}

// openResizeModal opens the resize modal and feeds it the flavor list.
func openResizeModal(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := m.openResize()
	for _, msg := range collectMsgs(cmd) {
		if _, ok := msg.(spinner.TickMsg); ok {
			continue
		}
		res, _ := m.Update(msg)
		m = res.(Model)
	}
	if !m.serverResize.Active {
		t.Fatal("resize modal not open")
	}
	return m
}

// nonSpinner runs cmd and returns its non-spinner messages.
func nonSpinner(cmd tea.Cmd) []tea.Msg {
	var out []tea.Msg
	for _, msg := range collectMsgs(cmd) {
		if _, ok := msg.(spinner.TickMsg); ok || msg == nil {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func TestResizeModalHonorsServerInflightLock(t *testing.T) {
	m, log := resizeModalFixture(t)

	// Another mutation of srv-a is pending.
	res, pause := m.Update(confirmed("pause", "srv-a"))
	m = res.(Model)
	if pause == nil {
		t.Fatal("pause not scheduled")
	}

	m = openResizeModal(t, m)
	res, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = res.(Model)
	if msgs := nonSpinner(cmd); len(msgs) != 0 {
		t.Fatalf("resize of a busy server produced %v", msgs)
	}
	if n := log.count("POST /servers/srv-a/action"); n != 0 {
		t.Fatalf("resize request sent for a busy server (%d actions)", n)
	}
	if !m.serverResize.Active || !strings.Contains(m.serverResize.View(), "in progress") {
		t.Fatalf("busy refusal not shown in the modal:\n%s", m.serverResize.View())
	}

	// Once the pause finishes the resize goes through and holds the lock.
	m, _ = deliver(t, m, pause)
	res, resize := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = res.(Model)
	if resize == nil {
		t.Fatal("resize not submitted after the lock was released")
	}
	res, conflict := m.Update(confirmed("pause", "srv-a"))
	m = res.(Model)
	if conflict != nil {
		t.Fatal("pause was scheduled while the resize was in flight")
	}

	var result tea.Msg
	for _, msg := range nonSpinner(resize) {
		if _, ok := msg.(actionResultMsg); ok {
			result = msg
		}
	}
	if result == nil {
		t.Fatal("resize command was not tracked as an action")
	}
	res, _ = m.Update(result)
	m = res.(Model)
	if n := log.count("POST /servers/srv-a/action"); n != 2 {
		t.Fatalf("server actions=%d, want pause + resize", n)
	}
	if m.serverResize.Active {
		t.Fatal("modal did not close after a successful resize")
	}
	if len(m.actions.inflight) != 0 {
		t.Fatalf("locks leaked: %v", m.actions.inflight)
	}
}
