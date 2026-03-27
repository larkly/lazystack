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
	if m.Err != "connection refused" {
		t.Errorf("Err = %q, want %q", m.Err, "connection refused")
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
