package shared

import (
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestTruncateID(t *testing.T) {
	uuid := "0f8e2c5a-1b3d-4e6f-8a9b-0c1d2e3f4a5b"
	cases := []struct {
		id   string
		n    int
		want string
	}{
		{"", 8, ""},
		{"a", 8, "a"},
		{"abcdefg", 8, "abcdefg"},
		{"abcdefgh", 8, "abcdefgh"},
		{uuid, 8, "0f8e2c5a"},
		{uuid, 12, "0f8e2c5a-1b3"},
		{"ÆØÅæøåéü漢字", 8, "ÆØÅæøåéü"},
		{uuid, 0, ""},
		{uuid, -3, ""},
	}
	for _, c := range cases {
		got := TruncateID(c.id, c.n)
		if got != c.want {
			t.Errorf("TruncateID(%q, %d) = %q, want %q", c.id, c.n, got, c.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("TruncateID(%q, %d) produced invalid UTF-8", c.id, c.n)
		}
	}
	if ShortID(uuid) != "0f8e2c5a" || ShortID("x") != "x" {
		t.Errorf("ShortID mismatch: %q %q", ShortID(uuid), ShortID("x"))
	}
}

func TestTruncateCells(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Bold(true).Render("router-éé漢字🙂-name") + " tail"
	inputs := []string{
		"plain-ascii-name",
		"漢字漢字漢字漢字",
		"emoji🙂🙂🙂🙂",
		"café café café",
		styled,
	}
	for _, in := range inputs {
		for w := -1; w <= 12; w++ {
			got := TruncateCells(in, w)
			if !utf8.ValidString(got) {
				t.Fatalf("TruncateCells(%q, %d) invalid UTF-8: %q", in, w, got)
			}
			budget := max(w, 0)
			if cw := lipgloss.Width(got); cw > budget {
				t.Fatalf("TruncateCells(%q, %d) width %d > %d: %q", in, w, cw, budget, got)
			}
			// Styled input must keep complete escape sequences: stripping
			// them leaves no stray ESC bytes.
			if strings.Contains(ansi.Strip(got), "\x1b") {
				t.Fatalf("TruncateCells(%q, %d) left a broken escape: %q", in, w, got)
			}
		}
	}
	if got := TruncateCells("short", 10); got != "short" {
		t.Errorf("fitting input changed: %q", got)
	}
	if got := TruncateCells("abcdefghij", 5); got != "abcd…" {
		t.Errorf("TruncateCells ascii = %q, want abcd…", got)
	}
}
