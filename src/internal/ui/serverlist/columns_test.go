package serverlist

import (
	"testing"
)

func TestComputeWidths_AllFit(t *testing.T) {
	cols := []Column{
		{Title: "A", MinWidth: 10, Flex: 1, Priority: 0},
		{Title: "B", MinWidth: 10, Flex: 1, Priority: 0},
	}
	cols = ComputeWidths(cols, 100)

	for _, c := range cols {
		if c.Hidden() {
			t.Error("no columns should be hidden at width 100")
		}
		if c.Width() < c.MinWidth {
			t.Errorf("column %s width %d < min %d", c.Title, c.Width(), c.MinWidth)
		}
	}
}

func TestComputeWidths_HidesLowPriority(t *testing.T) {
	cols := []Column{
		{Title: "A", MinWidth: 20, Flex: 0, Priority: 0},
		{Title: "B", MinWidth: 20, Flex: 0, Priority: 1},
		{Title: "C", MinWidth: 20, Flex: 0, Priority: 2},
	}
	// Total min = 60 + gaps(2) + padding(2) = 64, fits in 80
	// But at width 50: 60 + 4 = 64 > 50, must hide
	cols = ComputeWidths(cols, 50)

	if cols[0].Hidden() {
		t.Error("priority 0 column should not be hidden")
	}
	if cols[1].Hidden() {
		t.Error("priority 1 column should not be hidden")
	}
	if !cols[2].Hidden() {
		t.Error("priority 2 column should be hidden at width 50")
	}
}

func TestComputeWidths_FlexDistribution(t *testing.T) {
	cols := []Column{
		{Title: "A", MinWidth: 10, Flex: 1, Priority: 0},
		{Title: "B", MinWidth: 10, Flex: 3, Priority: 0},
	}
	cols = ComputeWidths(cols, 60)

	// Available = 60 - 2(padding) - 1(gap) = 57
	// Min total = 20, remaining = 37
	// A gets 37*1/4 = 9 extra, B gets 37*3/4 = 27 extra
	if cols[1].Width() <= cols[0].Width() {
		t.Errorf("flex 3 column (%d) should be wider than flex 1 column (%d)",
			cols[1].Width(), cols[0].Width())
	}
}

func TestComputeWidths_FixedColumns(t *testing.T) {
	cols := []Column{
		{Title: "A", MinWidth: 15, Flex: 0, Priority: 0},
		{Title: "B", MinWidth: 10, Flex: 2, Priority: 0},
	}
	cols = ComputeWidths(cols, 80)

	if cols[0].Width() != 15 {
		t.Errorf("fixed column should stay at MinWidth 15, got %d", cols[0].Width())
	}
	if cols[1].Width() <= 10 {
		t.Errorf("flex column should grow beyond MinWidth 10, got %d", cols[1].Width())
	}
}

func TestComputeWidths_NarrowTerminal(t *testing.T) {
	cols := DefaultColumns()
	cols = ComputeWidths(cols, 40)

	visibleCount := 0
	for _, c := range cols {
		if !c.Hidden() {
			visibleCount++
		}
	}
	if visibleCount == 0 {
		t.Error("at least one column should be visible")
	}
	if visibleCount == len(cols) {
		t.Error("some columns should be hidden at width 40")
	}
}
