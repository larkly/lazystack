package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/cloud"
	"github.com/larkly/lazystack/internal/network"
)

// Rename and clone need a selected security group, like every other
// security group action.
func TestSGRenameAndCloneNeedSelection(t *testing.T) {
	m := newTestModel("dev", false)
	m.client = &cloud.Client{}
	if m.secGroupView.SelectedGroupID() != "" {
		t.Fatal("setup: expected no selected security group")
	}
	for name, open := range map[string]func(Model) (Model, tea.Cmd){
		"rename": Model.openSGRename,
		"clone":  Model.openSGClone,
	} {
		next, cmd := open(m)
		if next.sgCreate.Active || cmd != nil {
			t.Fatalf("%s opened without a selected security group", name)
		}
	}
}

// Unnamed ports with short IDs must not panic the delete confirmation.
func TestPortDisplayNameHandlesShortIDs(t *testing.T) {
	for _, tc := range []struct {
		port network.Port
		want string
	}{
		{network.Port{ID: "abc"}, "abc"},
		{network.Port{ID: ""}, ""},
		{network.Port{ID: "0123456789abcdef"}, "01234567…"},
		{network.Port{ID: "0123456789abcdef", Name: "web"}, "web"},
	} {
		if got := portDisplayName(tc.port); got != tc.want {
			t.Errorf("portDisplayName(%q) = %q, want %q", tc.port.ID, got, tc.want)
		}
	}
}
