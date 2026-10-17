package serverrename

import (
	"strings"
	"testing"
)

// Enter submits this single-field modal, so its footer and status-bar
// hints say "enter save".
func TestFooterSaysEnterSave(t *testing.T) {
	m := New(nil, "srv-1", "web")
	m.SetSize(100, 30)
	if v := m.View(); !strings.Contains(v, "enter save") {
		t.Fatalf("footer does not say enter save:\n%s", v)
	}
	if h := m.Hints(); !strings.HasPrefix(h, "enter save") {
		t.Fatalf("hints=%q", h)
	}
}
