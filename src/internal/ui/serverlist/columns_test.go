package serverlist

import (
	"testing"
)

func TestComputeWidths_AllFit(t *testing.T) {
	cols := []Column{
		{Title: "A", MinWidth: 10, Flex: 1, Priority: 0},
		{Title: "B", MinWidth: 10, Flex: 1, Priority: 0},
	}
	cols = ComputeWidths(cols, 100, nil)

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
	cols = ComputeWidths(cols, 50, nil)

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
	cols = ComputeWidths(cols, 60, nil)

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
	cols = ComputeWidths(cols, 80, nil)

	if cols[0].Width() != 15 {
		t.Errorf("fixed column should stay at MinWidth 15, got %d", cols[0].Width())
	}
	if cols[1].Width() <= 10 {
		t.Errorf("flex column should grow beyond MinWidth 10, got %d", cols[1].Width())
	}
}

func TestComputeWidths_NameReservesMinWidth(t *testing.T) {
	cols := DefaultColumns()
	cols = ComputeWidths(cols, 120, nil)

	for _, c := range cols {
		if c.Key == "name" {
			if c.Hidden() {
				t.Fatal("name column should not be hidden at width 120")
			}
			if c.Width() < 20 {
				t.Errorf("name column width = %d, want >= 20", c.Width())
			}
			return
		}
	}
	t.Fatal("name column not found")
}

func TestComputeWidths_NameGrowsFasterThanIPv6(t *testing.T) {
	cols := DefaultColumns()
	cols = ComputeWidths(cols, 180, nil)

	var nameW, ipv6W int
	for _, c := range cols {
		if c.Hidden() {
			continue
		}
		switch c.Key {
		case "name":
			nameW = c.Width()
		case "ipv6":
			ipv6W = c.Width()
		}
	}
	if ipv6W == 0 {
		t.Skip("ipv6 hidden at this width")
	}
	if nameW <= ipv6W {
		t.Errorf("name width (%d) should exceed ipv6 width (%d) at width 180", nameW, ipv6W)
	}
}

func TestComputeWidths_ContentCapLimitsGrowth(t *testing.T) {
	cols := []Column{
		{Title: "A", MinWidth: 10, Flex: 1, Priority: 0, Key: "a"},
		{Title: "B", MinWidth: 10, Flex: 1, Priority: 0, Key: "b"},
	}
	// Plenty of space (200) for both. A's content caps at 15; B uncapped.
	cols = ComputeWidths(cols, 200, map[string]int{"a": 15})

	if cols[0].Width() != 15 {
		t.Errorf("capped column width = %d, want 15", cols[0].Width())
	}
	// B should absorb the space A didn't take.
	if cols[1].Width() <= 15 {
		t.Errorf("uncapped column width = %d, want > 15 (should absorb leftover)", cols[1].Width())
	}
}

func TestComputeWidths_ContentCapBelowMinWidth(t *testing.T) {
	cols := []Column{
		{Title: "A", MinWidth: 20, Flex: 1, Priority: 0, Key: "a"},
	}
	// Content is tiny; MinWidth still guarantees 20.
	cols = ComputeWidths(cols, 100, map[string]int{"a": 3})

	if cols[0].Width() < 20 {
		t.Errorf("column width = %d, must respect MinWidth 20 even with small content", cols[0].Width())
	}
}

func TestComputeWidths_UncappedWhenContentMissing(t *testing.T) {
	cols := []Column{
		{Title: "A", MinWidth: 10, Flex: 1, Priority: 0, Key: "a"},
	}
	// Empty content map: no caps, falls back to pure flex distribution.
	cols = ComputeWidths(cols, 100, map[string]int{})

	if cols[0].Width() <= 10 {
		t.Errorf("with empty maxContent, flex should still grow column, got %d", cols[0].Width())
	}
}

func TestComputeWidths_NarrowTerminal(t *testing.T) {
	cols := DefaultColumns()
	cols = ComputeWidths(cols, 40, nil)

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
