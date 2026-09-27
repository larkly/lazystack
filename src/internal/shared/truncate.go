package shared

import "github.com/charmbracelet/x/ansi"

// TruncateID returns at most the first n runes of a resource ID for display.
// It never panics on short, empty or non-ASCII IDs and never splits a UTF-8
// sequence. Use it only for display text; API calls must keep the full ID.
func TruncateID(id string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range id {
		if count == n {
			return id[:i]
		}
		count++
	}
	return id
}

// ShortID is the conventional 8-character display form of a resource ID.
func ShortID(id string) string {
	return TruncateID(id, 8)
}

// AbbrevID returns id unchanged when it is at most 8 characters long and
// otherwise its short form followed by an ellipsis.
func AbbrevID(id string) string {
	if short := ShortID(id); short != id {
		return short + "…"
	}
	return id
}

// TruncateCells shortens s to at most width terminal cells, ending with an
// ellipsis when anything was cut. It is aware of ANSI escape sequences and
// wide or combining characters, so styled text and Unicode stay valid.
func TruncateCells(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}
