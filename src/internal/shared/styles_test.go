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

func TestModalWidthClampsToTerminal(t *testing.T) {
	tests := []struct{ fixed, term, want int }{
		{55, 0, 55},   // not sized yet: keep the design width
		{55, 120, 55}, // wide terminal: design width
		{55, 61, 55},  // exactly fits with the margin
		{55, 40, 34},  // narrow: terminal minus margin
		{55, 20, 20},  // very narrow: never below the minimum
		{40, 10, 20},
	}
	for _, tt := range tests {
		if got := ModalWidth(tt.fixed, tt.term); got != tt.want {
			t.Errorf("ModalWidth(%d, %d) = %d, want %d", tt.fixed, tt.term, got, tt.want)
		}
	}
}
