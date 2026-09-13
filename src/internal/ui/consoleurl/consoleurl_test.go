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
