package shared

import "testing"

func TestGlyphFollowsPlainMode(t *testing.T) {
	prev := PlainMode
	t.Cleanup(func() { PlainMode = prev })

	PlainMode = false
	if got := Glyph("✓", "OK"); got != "✓" {
		t.Errorf("unicode mode: Glyph = %q, want %q", got, "✓")
	}
	PlainMode = true
	if got := Glyph("✓", "OK"); got != "OK" {
		t.Errorf("plain mode: Glyph = %q, want %q", got, "OK")
	}
}
