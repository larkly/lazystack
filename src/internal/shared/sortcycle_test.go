package shared

import "testing"

func TestNextSortCyclesColumnsAscending(t *testing.T) {
	s := SortState{Col: 0, Asc: false}
	s = NextSort(s, 3)
	if s != (SortState{Col: 1, Asc: true, Highlight: true}) {
		t.Fatalf("NextSort = %+v, want col 1 ascending highlighted", s)
	}
	s = NextSort(NextSort(s, 3), 3)
	if s.Col != 0 || !s.Asc {
		t.Fatalf("NextSort did not wrap to column 0 ascending: %+v", s)
	}
}

func TestNextSortWithoutColumnsKeepsState(t *testing.T) {
	cur := SortState{Col: 2, Asc: false}
	if got := NextSort(cur, 0); got != cur {
		t.Fatalf("NextSort with 0 columns = %+v, want unchanged %+v", got, cur)
	}
}

func TestReverseSortFlipsDirection(t *testing.T) {
	s := ReverseSort(SortState{Col: 2, Asc: true})
	if s != (SortState{Col: 2, Asc: false, Highlight: true}) {
		t.Fatalf("ReverseSort = %+v", s)
	}
	if s = ReverseSort(s); !s.Asc {
		t.Fatalf("second ReverseSort did not flip back: %+v", s)
	}
}

func TestSortIndicator(t *testing.T) {
	s := SortState{Col: 1, Asc: true}
	if s.Indicator(0) != "" || s.Indicator(1) != " ▲" {
		t.Fatalf("ascending indicators: %q %q", s.Indicator(0), s.Indicator(1))
	}
	s.Asc = false
	if s.Indicator(1) != " ▼" {
		t.Fatalf("descending indicator: %q", s.Indicator(1))
	}
}

func TestNewSortStateIsAscendingFirstColumn(t *testing.T) {
	if s := NewSortState(); s.Col != 0 || !s.Asc || s.Highlight {
		t.Fatalf("NewSortState = %+v", s)
	}
}

func TestSortHighlightCmdDeliversMessage(t *testing.T) {
	type done struct{}
	if SortHighlightCmd(done{}) == nil {
		t.Fatal("SortHighlightCmd returned nil")
	}
}

func TestRestoreCursor(t *testing.T) {
	ids := []string{"a", "b", "c"}
	cases := []struct {
		prev string
		want int
	}{
		{"a", 0}, {"c", 2}, {"gone", -1}, {"", -1},
	}
	for _, c := range cases {
		if got := RestoreCursor(ids, c.prev); got != c.want {
			t.Errorf("RestoreCursor(%q) = %d, want %d", c.prev, got, c.want)
		}
	}
	if got := RestoreCursor(nil, "a"); got != -1 {
		t.Errorf("RestoreCursor on empty list = %d, want -1", got)
	}
}

func TestClampCursor(t *testing.T) {
	cases := []struct{ cursor, n, want int }{
		{0, 0, 0}, {5, 0, 0}, {-1, 3, 0}, {1, 3, 1}, {7, 3, 2},
	}
	for _, c := range cases {
		if got := ClampCursor(c.cursor, c.n); got != c.want {
			t.Errorf("ClampCursor(%d, %d) = %d, want %d", c.cursor, c.n, got, c.want)
		}
	}
}
