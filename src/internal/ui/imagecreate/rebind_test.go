package imagecreate

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
)

// The background and large-file deny keys follow their bindings.
func TestUploadKeysFollowKeybindings(t *testing.T) {
	prevBg, prevDeny := shared.Keys.Background, shared.Keys.Deny
	t.Cleanup(func() { shared.Keys.Background, shared.Keys.Deny = prevBg, prevDeny })
	shared.Keys.Background = key.NewBinding(key.WithKeys("z"))
	shared.Keys.Deny = key.NewBinding(key.WithKeys("x"))

	m := Model{Active: true, uploading: true}
	m, _ = m.handleUploadingKey(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if !m.Active {
		t.Fatal("b still backgrounds the upload after rebinding")
	}
	m, _ = m.handleUploadingKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if m.Active {
		t.Fatal("rebound background key did not background the upload")
	}

	m = Model{Active: true, warnLargeFile: true}
	m, _ = m.handleLargeFileWarning(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !m.warnLargeFile {
		t.Fatal("n still dismisses the large-file warning after rebinding deny")
	}
	m, _ = m.handleLargeFileWarning(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.warnLargeFile {
		t.Fatal("rebound deny key did not dismiss the large-file warning")
	}
}
