package cloudpicker

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
)

func TestNew(t *testing.T) {
	clouds := []string{"alpha", "beta", "gamma"}
	m := New(clouds, nil)

	if len(m.clouds) != 3 {
		t.Errorf("clouds len = %d, want 3", len(m.clouds))
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
	if m.err != nil {
		t.Errorf("err = %v, want nil", m.err)
	}
}

func TestNew_WithError(t *testing.T) {
	e := errors.New("no clouds.yaml")
	m := New(nil, e)

	if m.err == nil {
		t.Error("expected error to be stored")
	}
	if m.err.Error() != "no clouds.yaml" {
		t.Errorf("err = %q, want %q", m.err.Error(), "no clouds.yaml")
	}
}

func TestCursorDown(t *testing.T) {
	m := New([]string{"a", "b", "c"}, nil)

	// Move down twice
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.cursor)
	}

	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2", m.cursor)
	}

	// Should not go past end
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (clamped)", m.cursor)
	}
}

func TestCursorUp(t *testing.T) {
	m := New([]string{"a", "b", "c"}, nil)

	// Should not go below 0
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped)", m.cursor)
	}

	// Move down then up
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
}

func TestEnter_SelectsCloud(t *testing.T) {
	m := New([]string{"prod", "staging", "dev"}, nil)

	// Move to "staging" (index 1)
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	if cmd == nil {
		t.Fatal("expected cmd from Enter")
	}
	msg := cmd()
	sel, ok := msg.(shared.CloudSelectedMsg)
	if !ok {
		t.Fatalf("expected CloudSelectedMsg, got %T", msg)
	}
	if sel.CloudName != "staging" {
		t.Errorf("CloudName = %q, want %q", sel.CloudName, "staging")
	}
}

func TestEnter_EmptyClouds(t *testing.T) {
	m := New([]string{}, nil)

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd != nil {
		t.Error("expected nil cmd for empty clouds list")
	}
}

func TestQuit(t *testing.T) {
	m := New([]string{"a"}, nil)

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestWindowSize(t *testing.T) {
	m := New([]string{"a"}, nil)

	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})

	if m.width != 120 {
		t.Errorf("width = %d, want 120", m.width)
	}
	if m.height != 50 {
		t.Errorf("height = %d, want 50", m.height)
	}
}

func TestPlainModeUsesASCIIGlyphs(t *testing.T) {
	prev := shared.PlainMode
	t.Cleanup(func() { shared.PlainMode = prev })
	shared.PlainMode = true

	m := New([]string{"alpha", "beta"}, nil)
	m.width, m.height = 80, 24
	got := m.View()
	for _, glyph := range []string{"▸", "↑/↓"} {
		if strings.Contains(got, glyph) {
			t.Errorf("plain mode view contains %q", glyph)
		}
	}
	if !strings.Contains(got, "> ") || !strings.Contains(got, "up/down navigate") {
		t.Errorf("plain mode view lacks ASCII cursor or hint: %q", got)
	}
	if h := m.Hints(); strings.Contains(h, "↑/↓") {
		t.Errorf("plain mode hints contain arrows: %q", h)
	}
}
