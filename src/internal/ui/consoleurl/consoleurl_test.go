package consoleurl

import (
	tea "charm.land/bubbletea/v2"
	"runtime"
	"strings"
	"testing"
)

func TestRenderResizeAndClose(t *testing.T) {
	url := "https://console.example/" + strings.Repeat("token", 20)
	m := New(url, "web")
	m.SetSize(80, 24)
	if !m.Active || m.Init() != nil {
		t.Fatal("initial state")
	}
	view := m.View()
	if !strings.Contains(view, "web") || !strings.Contains(view, "...") || strings.Contains(view, url) {
		t.Fatal(view)
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 50, Height: 20})
	if m.width != 50 || m.height != 20 {
		t.Fatal("resize")
	}
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if cmd != nil || m.status != "" || !m.Active {
		t.Fatal("unknown key")
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.Active || cmd != nil {
		t.Fatal("close")
	}
}
func TestMissingBrowserReportsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows launcher lookup differs")
	}
	t.Setenv("PATH", t.TempDir())
	m := New("https://console.example/", "web")
	m.SetSize(120, 30)
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || !m.Active || !strings.Contains(m.status, "Failed to open browser") || !strings.Contains(m.View(), "Failed to open browser") {
		t.Fatalf("status=%q", m.status)
	}
}

func TestOpenRejectsNonHTTPURLs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows launcher lookup differs")
	}
	// An empty PATH makes any launch attempt fail with "Failed to open
	// browser", so a refusal must be reported before a launcher is run.
	t.Setenv("PATH", t.TempDir())
	for _, u := range []string{
		"file:///etc/passwd",
		"javascript:alert(1)",
		"ssh://host",
		"-a Calculator",
		"--help",
		"",
		"https://",
		"not a url",
	} {
		m := New(u, "web")
		m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd != nil || !m.Active {
			t.Fatalf("%q: unexpected cmd/close", u)
		}
		if !strings.Contains(m.status, "Refusing to open") {
			t.Fatalf("%q: status=%q, want refusal", u, m.status)
		}
	}
}

func TestValidateURLAcceptsHTTPAndHTTPS(t *testing.T) {
	for _, u := range []string{
		"https://console.example/vnc?token=abc",
		"http://[2001:db8::1]:6080/vnc_auto.html",
		"HTTPS://Console.Example/",
	} {
		if err := validateURL(u); err != nil {
			t.Fatalf("%q rejected: %v", u, err)
		}
	}
}
