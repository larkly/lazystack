package servermetadata

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/larkly/lazystack/internal/shared"
)

func TestMetadataKeysFollowKeybindings(t *testing.T) {
	prevAdd, prevDel := shared.Keys.MetadataAdd, shared.Keys.MetadataDelete
	t.Cleanup(func() { shared.Keys.MetadataAdd, shared.Keys.MetadataDelete = prevAdd, prevDel })
	shared.Keys.MetadataAdd = key.NewBinding(key.WithKeys("+"))
	shared.Keys.MetadataDelete = key.NewBinding(key.WithKeys("-"))

	m, _ := newModel(t, map[string]string{"role": "db"})
	m = runes(m, "ad")
	if m.mode != "" {
		t.Fatalf("a/d still act after rebinding: mode=%q", m.mode)
	}
	m = runes(m, "-")
	if m.mode != "delete" {
		t.Fatalf("rebound delete key: mode=%q, want delete", m.mode)
	}
	m, _ = press(m, keyN)
	m = runes(m, "+")
	if m.mode != "add" {
		t.Fatalf("rebound add key: mode=%q, want add", m.mode)
	}
}
