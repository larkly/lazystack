package app

import (
	"testing"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/lbdetail"
)

func TestResourceActionMsgKeepsLBDetailOpen(t *testing.T) {
	m := newTestModel("dev", false)
	m.view = viewLBDetail
	m.statusBar.CurrentView = "lbdetail"
	m.lbDetail = lbdetail.New(nil, "lb-1")

	res, cmd := m.Update(shared.ResourceActionMsg{Action: "Created pool on", Name: "lb-1"})
	updated := res.(Model)

	if updated.view != viewLBDetail {
		t.Fatalf("view = %v, want LB detail", updated.view)
	}
	if updated.statusBar.CurrentView != "lbdetail" {
		t.Fatalf("statusBar.CurrentView = %q, want lbdetail", updated.statusBar.CurrentView)
	}
	if cmd == nil {
		t.Fatal("expected LB detail refresh command")
	}
}
