package volumelist

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/volumedetail"
)

func TestVolumeHintsUseConfiguredAttachBinding(t *testing.T) {
	prev := shared.Keys
	t.Cleanup(func() { shared.Keys = prev })
	shared.Keys.Attach = key.NewBinding(key.WithKeys("f9"), key.WithHelp("f9", "attach"))

	h := Model{}.Hints()
	if !strings.Contains(h, "f9 attach") || strings.Contains(h, "^a") {
		t.Errorf("list hints = %q, want the configured attach key", h)
	}
	// Tabs are reachable with number keys 1-9.
	if !strings.Contains(h, "1-9/←→ switch tab") {
		t.Errorf("list hints = %q, want the 1-9 tab range", h)
	}
	if d := (volumedetail.Model{}).Hints(); !strings.Contains(d, "f9 attach") || strings.Contains(d, "^a") {
		t.Errorf("detail hints = %q, want the configured attach key", d)
	}
}
