package shared

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// LoadErrorLine renders a failed load or refresh as the single warning line
// that list and detail views show under their title, while they keep showing
// the last data that did load. The message goes through ParseError so it is
// friendly and free of endpoint URLs, is joined onto one line and is cut to
// width so it never shifts the layout below it. It returns "" for a nil
// error; a width of 0 or less means the size is not known yet and leaves the
// line uncut.
func LoadErrorLine(err error, width int) string {
	if err == nil {
		return ""
	}
	msg := strings.Join(strings.Fields(ParseError(err).FriendlyMessage), " ")
	line := "  " + Glyph("⚠", "!") + " " + msg
	if width > 0 {
		line = TruncateCells(line, width)
	}
	return lipgloss.NewStyle().Foreground(ColorError).Render(line)
}
