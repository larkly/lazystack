package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/ui/lbview"
)

// R on the load balancer tab is handled by the app's global refresh, which
// is why the view itself has no R handler.
func TestRefreshKeyOnLBViewForcesRefresh(t *testing.T) {
	m := newTestModel("dev", false)
	m.view = viewLBView
	m.lbView = lbview.New(nil, time.Second)

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'R', Text: "R"}))
	if cmd == nil {
		t.Fatal("R on the load balancer view did not start a refresh")
	}
}
