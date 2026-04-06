package modal

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNewError(t *testing.T) {
	m := NewError("deleting server", errors.New("connection refused"))

	if m.Context != "deleting server" {
		t.Errorf("Context = %q, want %q", m.Context, "deleting server")
	}
	if m.FriendlyError == "" {
		t.Error("expected non-empty FriendlyError")
	}
	if m.RawError == "" {
		t.Error("expected non-empty RawError")
	}
	if m.ShowDetails != false {
		t.Error("expected ShowDetails to be false initially")
	}
}

func TestErrorCategories(t *testing.T) {
	m := NewError("test network", errors.New("connection refused"))
	if m.FriendlyError != "Network error — check connectivity and try again." {
		t.Errorf("expected network error message, got %q", m.FriendlyError)
	}
}

func TestEnterDismisses(t *testing.T) {
	m := NewError("test", errors.New("fail"))

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("expected cmd from enter key")
	}
	msg := cmd()
	if _, ok := msg.(ErrorDismissedMsg); !ok {
		t.Fatalf("expected ErrorDismissedMsg, got %T", msg)
	}
}

func TestEscDismisses(t *testing.T) {
	m := NewError("test", errors.New("fail"))

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if cmd == nil {
		t.Fatal("expected cmd from esc key")
	}
	msg := cmd()
	if _, ok := msg.(ErrorDismissedMsg); !ok {
		t.Fatalf("expected ErrorDismissedMsg, got %T", msg)
	}
}

func TestDToggleDetails(t *testing.T) {
	m := NewError("test", errors.New("fail"))

	// Initial state: not expanded.
	if m.ShowDetails != false {
		t.Error("expected ShowDetails to be false initially")
	}

	// Press 'd' to expand.
	m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	if cmd != nil {
		t.Error("expected nil cmd for d key")
	}
	if m.ShowDetails != true {
		t.Error("expected ShowDetails to be true after pressing d")
	}

	// Press 'd' again to collapse.
	m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	if cmd != nil {
		t.Error("expected nil cmd for d key")
	}
	if m.ShowDetails != false {
		t.Error("expected ShowDetails to be false after second d")
	}
}

func TestUpperDToggleDetails(t *testing.T) {
	m := NewError("test", errors.New("fail"))

	m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'D', Text: "D"}))
	if cmd != nil {
		t.Error("expected nil cmd for D key")
	}
	if m.ShowDetails != true {
		t.Error("expected ShowDetails to be true after pressing D")
	}
}

func TestOtherKeyIgnored(t *testing.T) {
	m := NewError("test", errors.New("fail"))

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	if cmd != nil {
		t.Error("expected nil cmd for unhandled key")
	}
}

func TestError_WindowSize(t *testing.T) {
	m := NewError("test", errors.New("fail"))

	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	if m.Width != 100 {
		t.Errorf("Width = %d, want 100", m.Width)
	}
	if m.Height != 40 {
		t.Errorf("Height = %d, want 40", m.Height)
	}
}
