package app

import (
	"testing"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/lbview"
)

func TestResourceActionMsgKeepsLBViewOpen(t *testing.T) {
	m := newTestModel("dev", false)
	m.view = viewLBView
	m.statusBar.CurrentView = "lbview"
	m.lbView = lbview.New(nil, 0)

	res, cmd := m.Update(shared.ResourceActionMsg{Action: "Created pool on", Name: "lb-1"})
	updated := res.(Model)

	if updated.view != viewLBView {
		t.Fatalf("view = %v, want LB view", updated.view)
	}
	if updated.statusBar.CurrentView != "lbview" {
		t.Fatalf("statusBar.CurrentView = %q, want lbview", updated.statusBar.CurrentView)
	}
	if cmd == nil {
		t.Fatal("expected LB view refresh command")
	}
}
