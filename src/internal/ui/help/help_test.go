package help

import (
	"testing"

	"github.com/gkampitakis/go-snaps/snaps"
)

func TestHelpOverlay_Render(t *testing.T) {
	m := New()
	m.Visible = true
	m.View = "serverlist"
	m.Width = 80
	m.Height = 24
	m.Open("serverlist")

	got := m.Render()
	snaps.MatchSnapshot(t, got)
}
