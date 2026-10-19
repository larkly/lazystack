package modal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNewConfirm(t *testing.T) {
	m := NewConfirm("delete", "srv-123", "my-server")

	if m.Action != "delete" {
		t.Errorf("Action = %q, want %q", m.Action, "delete")
	}
	if m.ServerID != "srv-123" {
		t.Errorf("ServerID = %q, want %q", m.ServerID, "srv-123")
	}
	if m.Name != "my-server" {
		t.Errorf("Name = %q, want %q", m.Name, "my-server")
	}
	if m.focused != 1 {
		t.Errorf("focused = %d, want 1 (cancel by default)", m.focused)
	}
}

func TestNewBulkConfirm(t *testing.T) {
	servers := []ServerRef{
		{ID: "s1", Name: "web-1"},
		{ID: "s2", Name: "web-2"},
		{ID: "s3", Name: "web-3"},
	}
	m := NewBulkConfirm("reboot", servers)

	if m.Action != "reboot" {
		t.Errorf("Action = %q, want %q", m.Action, "reboot")
	}
	if len(m.Servers) != 3 {
		t.Errorf("Servers len = %d, want 3", len(m.Servers))
	}
	if m.Name != "3 servers" {
		t.Errorf("Name = %q, want %q", m.Name, "3 servers")
	}
	if m.focused != 1 {
		t.Errorf("focused = %d, want 1 (cancel by default)", m.focused)
	}
}

// A bulk confirmation names the servers it acts on: up to five names,
// then a count of the rest.
func TestBulkConfirmBodyNamesServers(t *testing.T) {
	refs := func(names ...string) []ServerRef {
		out := make([]ServerRef, len(names))
		for i, n := range names {
			out[i] = ServerRef{ID: "id-" + n, Name: n}
		}
		return out
	}
	for _, tc := range []struct {
		action string
		refs   []ServerRef
		want   string
	}{
		{"delete", refs("a", "b", "c"), "Delete 3 servers: a, b, c"},
		{"soft reboot", refs("a", "b"), "Soft reboot 2 servers: a, b"},
		{"pause", refs("a", "b", "c", "d", "e"), "Pause 5 servers: a, b, c, d, e"},
		{"delete", refs("a", "b", "c", "d", "e", "f", "g"), "Delete 7 servers: a, b, c, d, e and 2 more"},
		{"stop", []ServerRef{{ID: "id-x"}, {ID: "id-y", Name: "y"}}, "Stop 2 servers: id-x, y"},
	} {
		m := NewBulkConfirm(tc.action, tc.refs)
		if m.Body != tc.want {
			t.Errorf("Body = %q, want %q", m.Body, tc.want)
		}
		m.SetSize(200, 40)
		if v := m.View(); !strings.Contains(v, tc.want) || strings.Contains(v, "servers\"") {
			t.Errorf("view does not show %q:\n%s", tc.want, v)
		}
	}
}

func TestConfirmKey(t *testing.T) {
	m := NewConfirm("delete", "srv-1", "web-1")

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'y', Text: "y"}))
	if cmd == nil {
		t.Fatal("expected cmd from y key")
	}
	msg := cmd()
	action, ok := msg.(ConfirmAction)
	if !ok {
		t.Fatalf("expected ConfirmAction, got %T", msg)
	}
	if !action.Confirm {
		t.Error("expected Confirm = true")
	}
	if action.Action != "delete" {
		t.Errorf("Action = %q, want %q", action.Action, "delete")
	}
	if action.ServerID != "srv-1" {
		t.Errorf("ServerID = %q, want %q", action.ServerID, "srv-1")
	}
}

func TestDenyKey(t *testing.T) {
	m := NewConfirm("delete", "srv-1", "web-1")

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	if cmd == nil {
		t.Fatal("expected cmd from n key")
	}
	msg := cmd()
	action, ok := msg.(ConfirmAction)
	if !ok {
		t.Fatalf("expected ConfirmAction, got %T", msg)
	}
	if action.Confirm {
		t.Error("expected Confirm = false")
	}
}

func TestBackKey(t *testing.T) {
	m := NewConfirm("delete", "srv-1", "web-1")

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if cmd == nil {
		t.Fatal("expected cmd from esc key")
	}
	msg := cmd()
	action, ok := msg.(ConfirmAction)
	if !ok {
		t.Fatalf("expected ConfirmAction, got %T", msg)
	}
	if action.Confirm {
		t.Error("expected Confirm = false on esc")
	}
}

func TestToggleFocus(t *testing.T) {
	m := NewConfirm("delete", "srv-1", "web-1")
	// starts at focused=1 (cancel)

	// Tab should toggle to 0
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if m.focused != 0 {
		t.Errorf("focused = %d, want 0 after tab", m.focused)
	}

	// Tab again should toggle back to 1
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if m.focused != 1 {
		t.Errorf("focused = %d, want 1 after second tab", m.focused)
	}

	// Arrow keys should also toggle
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	if m.focused != 0 {
		t.Errorf("focused = %d, want 0 after left arrow", m.focused)
	}
}

func TestEnter_FocusedConfirm(t *testing.T) {
	m := NewConfirm("delete", "srv-1", "web-1")
	m.focused = 0

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("expected cmd from enter on confirm")
	}
	msg := cmd()
	action, ok := msg.(ConfirmAction)
	if !ok {
		t.Fatalf("expected ConfirmAction, got %T", msg)
	}
	if !action.Confirm {
		t.Error("expected Confirm = true when focused on confirm button")
	}
}

func TestEnter_FocusedCancel(t *testing.T) {
	m := NewConfirm("delete", "srv-1", "web-1")
	// focused defaults to 1 (cancel)

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("expected cmd from enter on cancel")
	}
	msg := cmd()
	action, ok := msg.(ConfirmAction)
	if !ok {
		t.Fatalf("expected ConfirmAction, got %T", msg)
	}
	if action.Confirm {
		t.Error("expected Confirm = false when focused on cancel button")
	}
}

func TestDeleteVolumes_SpaceToggle(t *testing.T) {
	m := NewConfirm("delete", "srv-1", "web-1")
	m.VolumeIDs = []string{"vol-1", "vol-2"}

	if m.deleteVolumes {
		t.Error("deleteVolumes should start false")
	}

	// Space toggles on
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
	if !m.deleteVolumes {
		t.Error("deleteVolumes should be true after space")
	}

	// Space toggles off
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
	if m.deleteVolumes {
		t.Error("deleteVolumes should be false after second space")
	}
}

func TestDeleteVolumes_NoVolumes_SpaceIgnored(t *testing.T) {
	m := NewConfirm("delete", "srv-1", "web-1")
	// No VolumeIDs set

	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
	if m.deleteVolumes {
		t.Error("deleteVolumes should remain false when no VolumeIDs")
	}
}

func TestDeleteVolumes_ConfirmIncludesVolumeIDs(t *testing.T) {
	m := NewConfirm("delete", "srv-1", "web-1")
	m.VolumeIDs = []string{"vol-1", "vol-2"}

	// Toggle on
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))

	// Confirm
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'y', Text: "y"}))
	if cmd == nil {
		t.Fatal("expected cmd from confirm")
	}
	action, ok := cmd().(ConfirmAction)
	if !ok {
		t.Fatalf("expected ConfirmAction, got %T", cmd())
	}
	if !action.DeleteVolumes {
		t.Error("expected DeleteVolumes = true")
	}
	if len(action.VolumeIDs) != 2 {
		t.Errorf("VolumeIDs len = %d, want 2", len(action.VolumeIDs))
	}
}

func TestDeleteVolumes_ConfirmWithoutToggle(t *testing.T) {
	m := NewConfirm("delete", "srv-1", "web-1")
	m.VolumeIDs = []string{"vol-1"}

	// Confirm without toggling space
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'y', Text: "y"}))
	if cmd == nil {
		t.Fatal("expected cmd from confirm")
	}
	action := cmd().(ConfirmAction)
	if action.DeleteVolumes {
		t.Error("expected DeleteVolumes = false when not toggled")
	}
}

func TestConfirm_WindowSize(t *testing.T) {
	m := NewConfirm("delete", "srv-1", "web-1")

	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})

	if m.Width != 80 {
		t.Errorf("Width = %d, want 80", m.Width)
	}
	if m.Height != 30 {
		t.Errorf("Height = %d, want 30", m.Height)
	}
}
