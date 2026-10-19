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

// A hint wider than the space left of the cloud info is truncated, not
// dropped: at ordinary widths the bar still shows the start of the hint.
func TestLongHintIsTruncatedToFit(t *testing.T) {
	const hint = "↑↓ navigate • space select • enter detail • ^n create • ^d delete • ^o reboot • / filter • f save filter • F load filter • ? help"
	for _, width := range []int{80, 120} {
		m := New("test")
		m.Width = width
		m.CloudName = "production"
		m.ProjectName = "team-infrastructure"
		m.Region = "west"
		m.Hint = hint
		got := m.Render()
		if strings.Contains(got, "\n") {
			t.Fatalf("width %d: bar wrapped: %q", width, got)
		}
		if w := lipgloss.Width(got); w != width+shared.StyleStatusBar.GetHorizontalFrameSize() {
			t.Fatalf("width %d: bar is %d cells", width, w)
		}
		if !strings.Contains(got, "↑↓ navigate") || !strings.Contains(got, "…") {
			t.Fatalf("width %d: want the hint's start with an ellipsis, got %q", width, got)
		}
	}
}

func TestErrorGlyphFollowsPlainMode(t *testing.T) {
	prev := shared.PlainMode
	t.Cleanup(func() { shared.PlainMode = prev })
	m := New("test")
	m.Width = 80
	m.Error = "failure"

	shared.PlainMode = false
	if got := m.Render(); !strings.Contains(got, "⚠ failure") {
		t.Fatalf("unicode mode: %q", got)
	}
	shared.PlainMode = true
	if got := m.Render(); !strings.Contains(got, "! failure") || strings.Contains(got, "⚠") {
		t.Fatalf("plain mode: %q", got)
	}
}
