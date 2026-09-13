package vmpassword

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"
)

func TestPasswordDisplayAndCopyPriority(t *testing.T) {
	for _, tc := range []struct{ name, plain, encrypted, note, want, label string }{
		{"decrypted", "test-password", "encrypted-fixture", "", "test-password", "password"},
		{"encrypted", "", "encrypted-fixture", "Private key unavailable", "encrypted-fixture", "encrypted blob"},
		{"empty", "", "", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New("windows", "test-key", "/tmp/test-key", tc.plain, tc.encrypted, tc.note)
			m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
			value, label := m.copyValue()
			if value != tc.want || label != tc.label {
				t.Fatalf("copy=(%q,%q)", value, label)
			}
			for _, width := range []int{60, 100} {
				m.SetSize(width, 40)
				view := m.View()
				for _, s := range []string{"Admin Password", "windows", "test-key", tc.note} {
					if !strings.Contains(view, s) {
						t.Fatalf("missing %q", s)
					}
				}
				if tc.plain != "" {
					if !strings.Contains(view, tc.plain) || strings.Contains(view, tc.encrypted) {
						t.Fatal("plaintext should replace encrypted display")
					}
				} else if tc.encrypted != "" {
					if !strings.Contains(view, "Encrypted (base64)") || !strings.Contains(view, tc.encrypted) {
						t.Fatal("encrypted fallback missing")
					}
				} else if !strings.Contains(view, "No password set") || strings.Contains(view, "c: copy") {
					t.Fatal("empty password guidance")
				}
			}
			m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
			if !m.Active || cmd != nil {
				t.Fatal("unrecognized key closed modal")
			}
			m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
			if m.Active {
				t.Fatal("escape did not close")
			}
		})
	}
}

func TestCopyEmptyAndClipboardFailure(t *testing.T) {
	m := New("vm", "", "", "", "", "")
	m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Text: "c"}))
	if cmd != nil || m.status != "Nothing to copy" || !strings.Contains(m.View(), "Nothing to copy") {
		t.Fatal("empty copy feedback")
	}
	// Isolate from the user's clipboard: no clipboard executable can be found.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	m.plain = "test-only-password"
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Text: "c"}))
	if !strings.HasPrefix(m.status, "Clipboard error:") || !strings.Contains(m.View(), "Clipboard error:") {
		t.Fatalf("missing clipboard error: %q", m.status)
	}
}

func TestEncryptedDisplayTruncatesWithoutTruncatingCopy(t *testing.T) {
	blob := strings.Repeat("A", 100)
	m := New("vm", "", "", "", blob, "")
	m.SetSize(100, 40)
	if truncate(blob, 60) != strings.Repeat("A", 57)+"..." || !strings.Contains(m.View(), strings.Repeat("A", 57)) || strings.Contains(m.View(), blob) {
		t.Fatalf("long blob display not truncated: %q", m.View())
	}
	if value, _ := m.copyValue(); value != blob {
		t.Fatal("copy lost blob contents")
	}
}
