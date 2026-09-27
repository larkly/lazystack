package serverdetail

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/shared"
)

func TestVolumeHintsUseConfiguredBindings(t *testing.T) {
	prev := shared.Keys
	t.Cleanup(func() { shared.Keys = prev })
	shared.Keys.Attach = key.NewBinding(key.WithKeys("f9"), key.WithHelp("f9", "attach"))
	shared.Keys.AssignFIP = key.NewBinding(key.WithKeys("f10"), key.WithHelp("f10", "assign floating IP"))

	m := Model{focus: focusVolumes, blockClient: &gophercloud.ServiceClient{}}
	h := m.Hints()
	if !strings.Contains(h, "f9 attach volume") || !strings.Contains(h, "f10 assign FIP") {
		t.Errorf("hints do not show configured keys: %q", h)
	}
	if strings.Contains(h, "^a") || strings.Contains(h, "^b") {
		t.Errorf("hints still advertise ctrl+a/ctrl+b: %q", h)
	}

	m.blockClient = nil
	if h := m.Hints(); !strings.Contains(h, "f10 assign FIP") || strings.Contains(h, "^b") {
		t.Errorf("hints without block storage: %q", h)
	}
}
