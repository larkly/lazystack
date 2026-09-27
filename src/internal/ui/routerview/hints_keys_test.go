package routerview

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/larkly/lazystack/internal/shared"
)

func TestInterfaceHintsUseConfiguredAttachBinding(t *testing.T) {
	prev := shared.Keys
	t.Cleanup(func() { shared.Keys = prev })
	shared.Keys.Attach = key.NewBinding(key.WithKeys("f9"), key.WithHelp("f9", "attach"))

	m := Model{focus: FocusInterfaces}
	if h := m.Hints(); !strings.Contains(h, "f9 add interface") || strings.Contains(h, "^a") {
		t.Errorf("hints = %q, want the configured attach key", h)
	}
	if empty := m.renderInterfacesContent(80, 10); !strings.Contains(empty, "f9 to add") || strings.Contains(empty, "Ctrl+A") {
		t.Errorf("empty-state hint = %q, want the configured attach key", empty)
	}
}
