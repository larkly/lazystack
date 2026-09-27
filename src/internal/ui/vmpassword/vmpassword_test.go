package vmpassword

import (
	"fmt"

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

func TestCredentialsAreMaskedUntilRevealed(t *testing.T) {
	creds := []Credential{{Server: "alpha", Secret: "secret-a"}, {Server: "beta", Secret: "secret-b"}}
	m := NewCredentials("Rescue Passwords", "Shown once; not stored.", creds)
	m.SetSize(100, 40)
	view := m.View()
	for _, s := range []string{"Rescue Passwords", "alpha", "beta", "Shown once"} {
		if !strings.Contains(view, s) {
			t.Fatalf("missing %q in %q", s, view)
		}
	}
	if strings.Contains(view, "secret-a") || strings.Contains(view, "secret-b") {
		t.Fatal("secrets rendered before explicit reveal")
	}
	for _, c := range creds {
		for _, s := range []string{c.String(), fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c)} {
			if strings.Contains(s, c.Secret) {
				t.Fatalf("formatting leaks secret: %q", s)
			}
		}
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	view = m.View()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "secret-a") && !strings.Contains(line, "alpha") {
			t.Fatalf("secret shown without its server: %q", line)
		}
	}
	if !strings.Contains(view, "secret-a") || !strings.Contains(view, "secret-b") {
		t.Fatal("reveal did not show the secrets")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if value, label := m.copyValue(); value != "secret-b" || !strings.Contains(label, "beta") {
		t.Fatalf("copy=(%q,%q), want selected server's secret", value, label)
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	if strings.Contains(m.View(), "secret-b") {
		t.Fatal("hide did not mask again")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.Active {
		t.Fatal("escape did not close")
	}
}
