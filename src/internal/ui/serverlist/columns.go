package serverlist

import (
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/lipgloss/v2"
)

// Column defines a table column.
type Column struct {
	Title    string
	MinWidth int
	Flex     int // relative weight for distributing extra space; 0 = fixed at MinWidth
	Priority int // lower = more important, hidden when terminal too narrow
	Key      string
	width    int  // computed width
	hidden   bool // set by ComputeWidths
}

// DefaultColumns returns the standard server list columns, ordered by display position.
func DefaultColumns() []Column {
	return []Column{
		{Title: "Name", MinWidth: 20, Flex: 4, Priority: 0, Key: "name"},
		{Title: "Status", MinWidth: 16, Flex: 1, Priority: 0, Key: "status"},
		{Title: "IPv4", MinWidth: 12, Flex: 1, Priority: 1, Key: "ipv4"},
		{Title: "IPv6", MinWidth: 15, Flex: 1, Priority: 5, Key: "ipv6"},
		{Title: "Floating IP", MinWidth: 12, Flex: 1, Priority: 3, Key: "floating"},
		{Title: "Flavor", MinWidth: 10, Flex: 2, Priority: 1, Key: "flavor"},
		{Title: "Image", MinWidth: 10, Flex: 2, Priority: 2, Key: "image"},
		{Title: "Age", MinWidth: 5, Flex: 0, Priority: 1, Key: "age"},
		{Title: "Key", MinWidth: 8, Flex: 1, Priority: 4, Key: "key"},
	}
}

// ComputeWidths distributes available width across visible columns,
// hiding low-priority columns when there isn't enough space.
//
// maxContent optionally caps each column's growth to the longest actual value
// in the current data (keyed by Column.Key). When a column would otherwise
// grow past its content max, it stops at the cap and the leftover space
// redistributes to other flex columns. Pass nil (or an empty map) to disable
// capping and fall back to pure flex distribution.
func ComputeWidths(columns []Column, totalWidth int, maxContent map[string]int) []Column {
	// Reset visibility
	for i := range columns {
		columns[i].hidden = false
		columns[i].width = columns[i].MinWidth
	}

	// Progressively hide lowest-priority columns until they fit.
	// Find max priority.
	maxPrio := 0
	for _, c := range columns {
		if c.Priority > maxPrio {
			maxPrio = c.Priority
		}
	}

	for prio := maxPrio; prio >= 0; prio-- {
		if fitsWidth(columns, totalWidth) {
			break
		}
		// Hide all columns at this priority level
		for i := range columns {
			if columns[i].Priority == prio {
				columns[i].hidden = true
			}
		}
	}

	// Compute visible column stats.
	gaps := -1 // spaces between columns
	totalMin := 0
	for _, c := range columns {
		if c.hidden {
			continue
		}
		gaps++
		totalMin += c.MinWidth
	}
	if gaps < 0 {
		gaps = 0
	}

	available := totalWidth - 2 - gaps // 2 for left padding
	if available < 0 {
		available = 0
	}

	remaining := available - totalMin
	if remaining <= 0 {
		return columns
	}

	// Content-aware growth cap per column. -1 means no cap (unbounded).
	cap := func(col Column) int {
		if maxContent == nil {
			return -1
		}
		v, ok := maxContent[col.Key]
		if !ok || v <= 0 {
			return -1
		}
		if v < col.MinWidth {
			return col.MinWidth
		}
		return v
	}

	// Iteratively distribute remaining space by flex. Columns that would
	// exceed their content cap lock to the cap; their excess goes back into
	// the pool for other columns.
	locked := make([]bool, len(columns))
	for {
		totalFlex := 0
		for i, c := range columns {
			if c.hidden || c.Flex == 0 || locked[i] {
				continue
			}
			totalFlex += c.Flex
		}
		if totalFlex == 0 || remaining <= 0 {
			break
		}

		newlyLocked := false
		for i, c := range columns {
			if c.hidden || c.Flex == 0 || locked[i] {
				continue
			}
			share := remaining * c.Flex / totalFlex
			ceiling := cap(c)
			maxGrow := -1
			if ceiling >= 0 {
				maxGrow = ceiling - columns[i].width
				if maxGrow < 0 {
					maxGrow = 0
				}
			}
			if maxGrow >= 0 && share >= maxGrow {
				columns[i].width += maxGrow
				remaining -= maxGrow
				locked[i] = true
				newlyLocked = true
			}
		}

		if newlyLocked {
			continue
		}

		// No more caps hit: distribute the rest by flex to uncapped columns.
		for i, c := range columns {
			if c.hidden || c.Flex == 0 || locked[i] {
				continue
			}
			columns[i].width += remaining * c.Flex / totalFlex
		}
		break
	}

	return columns
}

func fitsWidth(columns []Column, totalWidth int) bool {
	needed := 2 // left padding
	first := true
	for _, c := range columns {
		if c.hidden {
			continue
		}
		if !first {
			needed++ // gap
		}
		first = false
		needed += c.MinWidth
	}
	return needed <= totalWidth
}

// Width returns the computed width for a column.
func (c Column) Width() int {
	if c.width > 0 {
		return c.width
	}
	return c.MinWidth
}

// Hidden returns whether the column is hidden due to lack of space.
func (c Column) Hidden() bool {
	return c.hidden
}

// StatusStyle returns the lipgloss style for a given server status.
func StatusStyle(status string) lipgloss.Style {
	color, ok := shared.StatusColors[status]
	if !ok {
		color = shared.ColorFg
	}
	return lipgloss.NewStyle().Foreground(color)
}
