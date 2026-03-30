package serverdetail

import (
	"strings"
	"testing"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
)

func TestShouldPollDetailAPIs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status string
		want   bool
	}{
		{status: "ACTIVE", want: true},
		{status: "SHUTOFF", want: false},
		{status: "SHELVED", want: false},
		{status: "SHELVED_OFFLOADED", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			if got := shouldPollDetailAPIs(tc.status); got != tc.want {
				t.Fatalf("shouldPollDetailAPIs(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestTickClearsConsoleErrorForIdleStatuses(t *testing.T) {
	t.Parallel()

	m := New(nil, nil, nil, "srv-1", 0)
	m.loading = false
	m.server = &compute.Server{ID: "srv-1", Status: "SHUTOFF"}
	m.consoleErr = "previous console error"
	m.actionsErr = "previous action error"
	m.interfacesErr = "previous interface error"

	updated, _ := m.Update(shared.TickMsg{})
	if updated.consoleErr != "" {
		t.Fatalf("consoleErr = %q, want empty", updated.consoleErr)
	}
	if updated.actionsErr != "" {
		t.Fatalf("actionsErr = %q, want empty", updated.actionsErr)
	}
	if updated.interfacesErr != "" {
		t.Fatalf("interfacesErr = %q, want empty", updated.interfacesErr)
	}
}

func TestPanelTitleShowsConsoleHotkey(t *testing.T) {
	t.Parallel()

	m := New(nil, nil, nil, "srv-1", 0)
	title := m.panelTitle(focusConsole)
	if !strings.Contains(title, "Console Log (L)") {
		t.Fatalf("panel title %q does not include Console Log (L)", title)
	}
}
