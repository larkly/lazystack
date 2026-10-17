package keypairlist

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/compute"
)

// A key pair just created must get the cursor once it is listed.
func TestSetHighlightSelectsKeypairOnceListed(t *testing.T) {
	m := New(nil, time.Hour)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.SetHighlight("zeta")
	m, _ = m.Update(keypairsLoadedMsg{keypairs: []compute.KeyPair{{Name: "alpha"}, {Name: "beta"}}})
	if kp := m.SelectedKeyPair(); kp == nil || kp.Name != "alpha" {
		t.Fatalf("cursor moved before the key pair was listed: %+v", kp)
	}
	m, _ = m.Update(keypairsLoadedMsg{keypairs: []compute.KeyPair{{Name: "alpha"}, {Name: "beta"}, {Name: "zeta"}}})
	if kp := m.SelectedKeyPair(); kp == nil || kp.Name != "zeta" {
		t.Fatalf("cursor on %+v, want the highlighted key pair", kp)
	}
	m.cursor = 0
	m, _ = m.Update(keypairsLoadedMsg{keypairs: []compute.KeyPair{{Name: "alpha"}, {Name: "beta"}, {Name: "zeta"}}})
	if kp := m.SelectedKeyPair(); kp == nil || kp.Name != "alpha" {
		t.Fatalf("cursor on %+v after refresh, want alpha", kp)
	}
}
