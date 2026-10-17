package shared

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/gophercloud/gophercloud/v2"
)

func TestLoadErrorLineNilIsEmpty(t *testing.T) {
	if got := LoadErrorLine(nil, 80); got != "" {
		t.Fatalf("LoadErrorLine(nil) = %q, want empty", got)
	}
}

func TestLoadErrorLineUsesFriendlyMessageAndGlyph(t *testing.T) {
	prev := PlainMode
	t.Cleanup(func() { PlainMode = prev })

	err := gophercloud.ErrUnexpectedResponseCode{Actual: 503, URL: "https://nova.example/v2.1/servers"}
	friendly := ParseError(err).FriendlyMessage

	PlainMode = false
	got := ansi.Strip(LoadErrorLine(err, 200))
	if !strings.Contains(got, "⚠ "+friendly) {
		t.Fatalf("unicode line = %q, want warning glyph and %q", got, friendly)
	}
	if strings.Contains(got, "nova.example") {
		t.Fatalf("line leaks the raw endpoint: %q", got)
	}

	PlainMode = true
	got = ansi.Strip(LoadErrorLine(err, 200))
	if !strings.Contains(got, "! "+friendly) || strings.Contains(got, "⚠") {
		t.Fatalf("plain line = %q, want ASCII glyph and %q", got, friendly)
	}
}

// The error line sits in a fixed slot of the view's layout, so it must stay
// on one line however long or multi-line the message is.
func TestLoadErrorLineIsOneLineWithinWidth(t *testing.T) {
	err := errors.New("first line\nsecond line " + strings.Repeat("x", 200))
	got := LoadErrorLine(err, 40)
	if strings.Contains(got, "\n") {
		t.Fatalf("line contains a newline: %q", got)
	}
	if w := ansi.StringWidth(got); w > 40 {
		t.Fatalf("line is %d cells wide, want at most 40", w)
	}
	if !strings.Contains(ansi.Strip(got), "first line second line") {
		t.Fatalf("line = %q, want the message joined onto one line", ansi.Strip(got))
	}
}
