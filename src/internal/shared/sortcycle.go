package shared

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// SortHighlightDuration is how long a list highlights its sort column after
// the sort changed.
const SortHighlightDuration = 1500 * time.Millisecond

// SortState is a list's sort column (an index into the columns it can sort
// by), its direction and whether the column header is briefly highlighted
// after a change.
type SortState struct {
	Col       int
	Asc       bool
	Highlight bool
}

// NewSortState sorts by the first column, ascending.
func NewSortState() SortState {
	return SortState{Asc: true}
}

// NextSort moves to the next of cols sortable columns, wrapping around, and
// sorts it ascending. With no sortable columns the state is unchanged.
func NextSort(cur SortState, cols int) SortState {
	if cols <= 0 {
		return cur
	}
	return SortState{Col: (cur.Col + 1) % cols, Asc: true, Highlight: true}
}

// ReverseSort flips the direction of the current sort column.
func ReverseSort(cur SortState) SortState {
	return SortState{Col: cur.Col, Asc: !cur.Asc, Highlight: true}
}

// Indicator is the arrow shown after the title of column col: the sort
// direction on the sort column and nothing elsewhere.
func (s SortState) Indicator(col int) string {
	if col != s.Col {
		return ""
	}
	if s.Asc {
		return " ▲"
	}
	return " ▼"
}

// SortHighlightCmd delivers msg once the sort highlight should end; the
// list then clears SortState.Highlight.
func SortHighlightCmd(msg tea.Msg) tea.Cmd {
	return tea.Tick(SortHighlightDuration, func(time.Time) tea.Msg { return msg })
}

// RestoreCursor returns the index of prevID in ids, the row IDs of a list in
// display order, so the cursor stays on the same item after a refresh,
// filter or sort. It returns -1 when prevID is empty or no longer listed;
// the caller then keeps its cursor position clamped with ClampCursor.
func RestoreCursor(ids []string, prevID string) int {
	if prevID == "" {
		return -1
	}
	for i, id := range ids {
		if id == prevID {
			return i
		}
	}
	return -1
}

// ClampCursor keeps cursor within a list of n rows (0 for an empty list).
func ClampCursor(cursor, n int) int {
	return max(0, min(cursor, n-1))
}
