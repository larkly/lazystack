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
		{Title: "Name", MinWidth: 10, Flex: 2, Priority: 0, Key: "name"},
		{Title: "Status", MinWidth: 20, Flex: 0, Priority: 0, Key: "status"},
		{Title: "IPv4", MinWidth: 12, Flex: 1, Priority: 1, Key: "ipv4"},
		{Title: "IPv6", MinWidth: 20, Flex: 4, Priority: 5, Key: "ipv6"},
		{Title: "Floating IP", MinWidth: 12, Flex: 1, Priority: 3, Key: "floating"},
		{Title: "Flavor", MinWidth: 10, Flex: 2, Priority: 1, Key: "flavor"},
		{Title: "Image", MinWidth: 10, Flex: 2, Priority: 2, Key: "image"},
		{Title: "Age", MinWidth: 5, Flex: 0, Priority: 1, Key: "age"},
		{Title: "Key", MinWidth: 8, Flex: 1, Priority: 4, Key: "key"},
	}
}

// ComputeWidths distributes available width across visible columns,
// hiding low-priority columns when there isn't enough space.
func ComputeWidths(columns []Column, totalWidth int) []Column {
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

	// Now distribute space among visible columns
	gaps := -1 // spaces between columns
	totalMin := 0
	totalFlex := 0
	for _, c := range columns {
		if c.hidden {
			continue
		}
		gaps++
		totalMin += c.MinWidth
		totalFlex += c.Flex
	}
	if gaps < 0 {
		gaps = 0
	}

	available := totalWidth - 2 - gaps // 2 for left padding
	if available < 0 {
		available = 0
	}

	remaining := available - totalMin
	if remaining > 0 && totalFlex > 0 {
		for i := range columns {
			if columns[i].hidden || columns[i].Flex == 0 {
				continue
			}
			extra := remaining * columns[i].Flex / totalFlex
			columns[i].width += extra
		}
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
