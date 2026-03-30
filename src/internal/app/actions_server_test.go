package app

import (
	"testing"

	"github.com/larkly/lazystack/internal/ui/modal"
)

func TestResolveToggleAction(t *testing.T) {
	tests := []struct {
		name   string
		toggle string
		status string
		locked bool
		want   string
	}{
		{name: "pause paused", toggle: "pause/unpause", status: "PAUSED", want: "unpause"},
		{name: "pause active", toggle: "pause/unpause", status: "ACTIVE", want: "pause"},
		{name: "suspend suspended", toggle: "suspend/resume", status: "SUSPENDED", want: "resume"},
		{name: "suspend active", toggle: "suspend/resume", status: "ACTIVE", want: "suspend"},
		{name: "shelve shelved", toggle: "shelve/unshelve", status: "SHELVED", want: "unshelve"},
		{name: "shelve offloaded", toggle: "shelve/unshelve", status: "SHELVED_OFFLOADED", want: "unshelve"},
		{name: "shelve active", toggle: "shelve/unshelve", status: "ACTIVE", want: "shelve"},
		{name: "stop shutoff", toggle: "stop/start", status: "SHUTOFF", want: "start"},
		{name: "stop active", toggle: "stop/start", status: "ACTIVE", want: "stop"},
		{name: "lock true", toggle: "lock/unlock", locked: true, want: "unlock"},
		{name: "lock false", toggle: "lock/unlock", locked: false, want: "lock"},
		{name: "rescue mode", toggle: "rescue/unrescue", status: "RESCUE", want: "unrescue"},
		{name: "rescue active", toggle: "rescue/unrescue", status: "ACTIVE", want: "rescue"},
		{name: "unknown toggle passthrough", toggle: "soft reboot", status: "ACTIVE", want: "soft reboot"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveToggleAction(tc.toggle, tc.status, tc.locked)
			if got != tc.want {
				t.Fatalf("resolveToggleAction(%q,%q,%v)=%q, want %q", tc.toggle, tc.status, tc.locked, got, tc.want)
			}
		})
	}
}

func TestCountBulkActions_UsesPerServerActionOrFallback(t *testing.T) {
	servers := []modal.ServerRef{
		{ID: "s1", Name: "one", Action: "start"},
		{ID: "s2", Name: "two", Action: "stop"},
		{ID: "s3", Name: "three", Action: ""},
	}
	got := countBulkActions(servers, "stop")
	if got["start"] != 1 {
		t.Fatalf("start count=%d, want 1", got["start"])
	}
	if got["stop"] != 2 {
		t.Fatalf("stop count=%d, want 2", got["stop"])
	}
}

func TestFormatActionCounts(t *testing.T) {
	counts := map[string]int{
		"stop":  4,
		"start": 2,
	}
	got := formatActionCounts(counts)
	want := "start:2, stop:4"
	if got != want {
		t.Fatalf("formatActionCounts=%q, want %q", got, want)
	}
}
