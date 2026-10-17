package consolelog

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
)

func TestJumpKeysFollowKeybindings(t *testing.T) {
	prevStart, prevEnd := shared.Keys.JumpStart, shared.Keys.JumpEnd
	t.Cleanup(func() { shared.Keys.JumpStart, shared.Keys.JumpEnd = prevStart, prevEnd })
	shared.Keys.JumpStart = key.NewBinding(key.WithKeys("home"))
	shared.Keys.JumpEnd = key.NewBinding(key.WithKeys("end"))

	m := New(nil, "server", "web")
	m.SetSize(20, 5)
	m, _ = m.Update(consoleLoadedMsg{"zero\none\ntwo\nthree\nfour"})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if m.scroll != 3 {
		t.Fatalf("g still jumps after rebinding: scroll=%d", m.scroll)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if m.scroll != 0 {
		t.Fatalf("home: scroll=%d, want 0", m.scroll)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if m.scroll != 3 {
		t.Fatalf("end: scroll=%d, want 3", m.scroll)
	}
}
