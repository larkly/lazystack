package imageview

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func typeKeys(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func cursorOn(t *testing.T, m Model, name string) Model {
	t.Helper()
	for i := 0; i < 20; i++ {
		if im := m.SelectedImage(); im != nil && im.Name == name {
			return m
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	t.Fatalf("cursor never reached %q", name)
	return m
}

func selectedName(m Model) string {
	if im := m.SelectedImage(); im != nil {
		return im.Name
	}
	return ""
}

// Typing a search and pressing Enter keeps the cursor on the image it was on
// while that image still matches; it only falls back to the top when the
// image is filtered out.
func TestSearchKeepsCursorOnSelectedImage(t *testing.T) {
	m := loadedModel(namedImages("web-1", "db-1", "web-2", "db-2")...)
	m = cursorOn(t, m, "web-2")

	m = typeKeys(m, "/")
	for _, r := range "web" {
		m = typeKeys(m, string(r))
		if got := selectedName(m); got != "web-2" {
			t.Fatalf("after typing %q the cursor is on %q, want web-2", m.searchFilter, got)
		}
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := selectedName(m); got != "web-2" {
		t.Fatalf("Enter moved the cursor to %q, want web-2", got)
	}

	m = typeKeys(m, "/")
	m = typeKeys(m, "x")
	if m.cursor != 0 {
		t.Fatalf("cursor=%d after the selected image was filtered out, want 0", m.cursor)
	}
}
