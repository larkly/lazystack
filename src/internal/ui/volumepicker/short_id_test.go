package volumepicker

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/testutil"
	"github.com/larkly/lazystack/internal/volume"
)

func TestUnnamedShortIDVolumesDoNotPanic(t *testing.T) {
	for _, id := range []string{"", "a", "abcdefg", "abcdefgh", "0f8e2c5a-1b3d-4e6f-8a9b-0c1d2e3f4a5b", "卷卷卷卷卷卷卷卷卷卷卷卷卷"} {
		m := New(nil, nil, "server-id", "web")
		m.SetSize(80, 20)
		m, _ = m.Update(volumesLoadedMsg{volumes: []volume.Volume{{ID: id, Status: "available", Size: 1}}})
		testutil.AssertRenderSafe(t, m.View(), 0)
		// Selecting builds the attach command, which names the volume.
		m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatalf("id %q: no attach command", id)
		}
		_ = m
	}
}
