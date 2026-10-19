package imagedownload

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
)

func TestBackgroundFollowsKeybinding(t *testing.T) {
	prev := shared.Keys.Background
	t.Cleanup(func() { shared.Keys.Background = prev })
	shared.Keys.Background = key.NewBinding(key.WithKeys("z"))

	m := Model{Active: true, downloading: true}
	m, _ = m.handleDownloadingKey(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if !m.Active {
		t.Fatal("b still backgrounds the download after rebinding")
	}
	m, _ = m.handleDownloadingKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if m.Active {
		t.Fatal("rebound background key did not background the download")
	}
}
