package serverlist

import (
	"github.com/bosse/lazystack/internal/shared"
	"charm.land/lipgloss/v2"
)

// Column defines a table column.
type Column struct {
	Title string
	Width int
	Key   string
}

// DefaultColumns returns the standard server list columns.
func DefaultColumns() []Column {
	return []Column{
		{Title: "Name", Width: 25, Key: "name"},
		{Title: "Status", Width: 12, Key: "status"},
		{Title: "IP", Width: 16, Key: "ip"},
		{Title: "Flavor", Width: 15, Key: "flavor"},
		{Title: "Key", Width: 15, Key: "key"},
		{Title: "ID", Width: 36, Key: "id"},
	}
}

// StatusStyle returns the lipgloss style for a given server status.
func StatusStyle(status string) lipgloss.Style {
	color, ok := shared.StatusColors[status]
	if !ok {
		color = shared.ColorFg
	}
	return lipgloss.NewStyle().Foreground(color)
}
