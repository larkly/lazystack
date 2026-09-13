package statusbar

import (
	"charm.land/lipgloss/v2"
	"github.com/larkly/lazystack/internal/shared"
	"strings"
	"testing"
)

func TestHintPriorityAndWidth(t *testing.T) {
	m := New("test")
	m.Width = 120
	if m.CurrentView != "cloudpicker" || m.Version != "test" || !strings.Contains(m.Render(), "Select a cloud") {
		t.Fatal("initial state")
	}
	m.CloudName = "production"
	m.Region = "west"
	m.ProjectName = "team"
	m.Hint = "ordinary"
	m.StickyHint = "sticky"
	m.Error = "failure"
	for _, want := range []string{"failure", "sticky", "ordinary"} {
		got := m.Render()
		if !strings.Contains(got, want) || !strings.Contains(got, "production") || !strings.Contains(got, "team") || !strings.Contains(got, "west") {
			t.Fatal(got)
		}
		if lipgloss.Width(got) != 120+shared.StyleStatusBar.GetHorizontalFrameSize() {
			t.Fatalf("width=%d", lipgloss.Width(got))
		}
		if want == "failure" && strings.Contains(got, "sticky") || want == "sticky" && strings.Contains(got, "ordinary") {
			t.Fatal("priority", got)
		}
		if want == "failure" {
			m.Error = ""
		} else {
			m.StickyHint = ""
		}
	}
	m.ProjectName = ""
	if strings.Contains(m.Render(), "project:") {
		t.Fatal("empty project label")
	}
	m.Width = 1
	if strings.Contains(m.Render(), "ordinary") {
		t.Fatal("overflow hint not hidden")
	}
	m.CloudName = ""
	m.Hint = ""
	m.Width = 10
	if lipgloss.Width(m.Render()) != 10+shared.StyleStatusBar.GetHorizontalFrameSize() {
		t.Fatal("blank bar width")
	}
}
