package testutil

import (
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// UnicodeName is a display name mixing CJK, emoji and a combining accent,
// long enough to need truncation in narrow table cells.
const UnicodeName = "名前🙂-cafe\u0301-漢字漢字-データベース-サーバー-🙂🙂"

// AssertRenderSafe fails the test if a rendered view is not valid UTF-8,
// contains a truncated ANSI escape sequence, or (when maxWidth > 0) has a
// line wider than maxWidth terminal cells.
func AssertRenderSafe(t *testing.T, view string, maxWidth int) {
	t.Helper()
	if !utf8.ValidString(view) {
		t.Fatalf("view is not valid UTF-8:\n%q", view)
	}
	if strings.Contains(ansi.Strip(view), "\x1b") {
		t.Fatalf("view contains a broken escape sequence:\n%q", view)
	}
	if maxWidth <= 0 {
		return
	}
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > maxWidth {
			t.Fatalf("line is %d cells wide, budget %d:\n%s", w, maxWidth, line)
		}
	}
}
