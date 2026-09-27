package volumedetail

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/volume"
)

func loadedModel(t *testing.T, metaKeys int, height int) Model {
	t.Helper()
	meta := make(map[string]string, metaKeys)
	for i := 0; i < metaKeys; i++ {
		meta[fmt.Sprintf("key%02d", i)] = "value"
	}
	m := New(nil, nil, "vol-1")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: height})
	m, _ = m.Update(volumeDetailLoadedMsg{vol: &volume.Volume{
		ID: "vol-1", Name: "data", Status: "available", Size: 10, Metadata: meta,
	}})
	return m
}

func press(m Model, code rune, n int) Model {
	for i := 0; i < n; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: code})
	}
	return m
}

func TestScrollIsClampedInUpdate(t *testing.T) {
	m := loadedModel(t, 20, 15)
	m = press(m, tea.KeyDown, 100)
	bottom := m.View()
	scrollAtBottom := m.scroll

	m = press(m, tea.KeyDown, 1)
	if m.scroll != scrollAtBottom {
		t.Fatalf("scroll grew past the end: %d -> %d", scrollAtBottom, m.scroll)
	}

	m = press(m, tea.KeyUp, 1)
	if m.scroll != scrollAtBottom-1 {
		t.Fatalf("one Up after overscroll: scroll=%d, want %d", m.scroll, scrollAtBottom-1)
	}
	if m.View() == bottom {
		t.Fatal("one Up after overscroll did not move the view")
	}
	if !strings.Contains(bottom, "key19") {
		t.Fatalf("bottom view does not show the last line:\n%s", bottom)
	}
}

func TestPageDownIsClampedInUpdate(t *testing.T) {
	m := loadedModel(t, 20, 15)
	m = press(m, tea.KeyPgDown, 10)
	bottom := m.scroll
	m = press(m, tea.KeyDown, 100)
	if m.scroll != bottom {
		t.Fatalf("PageDown did not clamp: %d vs %d", bottom, m.scroll)
	}
	m = press(m, tea.KeyPgUp, 1)
	if m.scroll >= bottom {
		t.Fatalf("PageUp after overscroll did not scroll up: %d", m.scroll)
	}
}

func TestScrollStaysZeroWhenContentFits(t *testing.T) {
	m := loadedModel(t, 0, 50)
	m = press(m, tea.KeyDown, 5)
	if m.scroll != 0 {
		t.Fatalf("scroll=%d, want 0 when all content fits", m.scroll)
	}
}

func TestReloadWithLessContentReclampsScroll(t *testing.T) {
	m := loadedModel(t, 20, 15)
	m = press(m, tea.KeyDown, 100)
	m, _ = m.Update(volumeDetailLoadedMsg{vol: &volume.Volume{ID: "vol-1", Name: "data"}})
	if m.scroll != 0 {
		t.Fatalf("scroll=%d after reload with short content, want 0", m.scroll)
	}
}

// Background ticks must not stack volume fetches, and a slow older response
// must never overwrite the result of a newer fetch.
func TestTickRefreshesDoNotOverlapOrRegress(t *testing.T) {
	m := loadedModel(t, 0, 40)

	m, first := m.Update(shared.TickMsg{})
	if first == nil {
		t.Fatal("tick did not fetch")
	}
	staleSeq := m.refresh.Seq()
	if _, again := m.Update(shared.TickMsg{}); again != nil {
		t.Fatal("tick started a second fetch while one was in flight")
	}

	m.ForceRefresh()
	m, _ = m.Update(volumeDetailLoadedMsg{seq: m.refresh.Seq(), vol: &volume.Volume{ID: "vol-1", Status: "in-use"}})
	m, _ = m.Update(volumeDetailLoadedMsg{seq: staleSeq, vol: &volume.Volume{ID: "vol-1", Status: "available"}})
	m, _ = m.Update(volumeDetailErrMsg{seq: staleSeq, err: fmt.Errorf("stale")})
	if m.volume.Status != "in-use" || m.err != "" {
		t.Fatalf("stale response applied: status=%q err=%q", m.volume.Status, m.err)
	}
	if _, next := m.Update(shared.TickMsg{}); next == nil {
		t.Fatal("tick blocked after the newest fetch completed")
	}
}
