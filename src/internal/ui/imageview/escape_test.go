package imageview

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestEscapeClearsSearchThenSelection(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.SetSize(120, 40)
	m, _ = m.Update(imagesLoadedMsg{images: namedImages("alpha", "beta")})
	m.selected["id-alpha"] = true
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.FilterValue() != "a" {
		t.Fatalf("setup: filter = %q", m.FilterValue())
	}

	if !m.Escape() || m.FilterValue() != "" || len(m.selected) != 1 {
		t.Fatalf("first escape: filter=%q selected=%d, want filter cleared and selection kept", m.FilterValue(), len(m.selected))
	}
	if !m.Escape() || len(m.selected) != 0 {
		t.Fatalf("second escape: selected=%d, want cleared", len(m.selected))
	}
	if m.Escape() {
		t.Fatal("third escape reported handled with nothing left to clear")
	}
}
