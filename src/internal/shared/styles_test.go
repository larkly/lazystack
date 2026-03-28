package shared

import "testing"

func TestStatusIcon_KnownStates(t *testing.T) {
	PlainMode = false
	tests := []struct {
		status string
		want   string
	}{
		{"ACTIVE", "● "},
		{"RUNNING", "● "},
		{"available", "● "},
		{"ONLINE", "● "},
		{"active", "● "},
		{"BUILD", "▲ "},
		{"RESIZE", "▲ "},
		{"NOSTATE", "▲ "},
		{"ERROR", "✘ "},
		{"CRASHED", "✘ "},
		{"DELETED", "✘ "},
		{"OFFLINE", "✘ "},
		{"SHUTOFF", "○ "},
		{"SHUTDOWN", "○ "},
		{"DOWN", "○ "},
		{"REBOOT", "↻ "},
		{"HARD_REBOOT", "↻ "},
		{"in-use", "↻ "},
		{"PAUSED", "■ "},
		{"SUSPENDED", "■ "},
		{"SHELVED", "■ "},
		{"deactivated", "■ "},
	}
	for _, tt := range tests {
		got := StatusIcon(tt.status)
		if got != tt.want {
			t.Errorf("StatusIcon(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestStatusIcon_PendingPrefix(t *testing.T) {
	PlainMode = false
	tests := []string{"PENDING_CREATE", "PENDING_UPDATE", "PENDING_DELETE"}
	for _, s := range tests {
		got := StatusIcon(s)
		if got != "▲ " {
			t.Errorf("StatusIcon(%q) = %q, want %q", s, got, "▲ ")
		}
	}
}

func TestStatusIcon_UnknownState(t *testing.T) {
	PlainMode = false
	got := StatusIcon("UNKNOWN_STATE")
	if got != "" {
		t.Errorf("StatusIcon(UNKNOWN_STATE) = %q, want empty", got)
	}
}

func TestStatusIcon_PlainMode(t *testing.T) {
	PlainMode = true
	defer func() { PlainMode = false }()
	got := StatusIcon("ACTIVE")
	if got != "" {
		t.Errorf("StatusIcon in PlainMode = %q, want empty", got)
	}
}
