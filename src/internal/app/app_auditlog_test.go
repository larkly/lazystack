package app

import (
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// collectMsgs runs cmd (expanding batches) and returns the produced messages.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, collectMsgs(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestOpenAuditLogAnimatesSpinnerWhileLoading(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel("dev", false)
	m.view = viewServerList
	m, cmd := m.openAuditLog()

	var tick tea.Msg
	for _, msg := range collectMsgs(cmd) {
		if _, ok := msg.(spinner.TickMsg); ok {
			tick = msg
		}
	}
	if tick == nil {
		t.Fatal("opening the audit log did not start its spinner")
	}
	res, next := m.Update(tick)
	if next == nil {
		t.Fatal("spinner tick was not routed to the loading audit log")
	}
	m = res.(Model)
	if m.view != viewAuditLog {
		t.Fatalf("view = %v, want audit log", m.view)
	}
}
