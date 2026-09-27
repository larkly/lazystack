package volumedetail

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
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
	m, _ = m.Update(volumeDetailLoadedMsg{volumeID: "vol-1", vol: &volume.Volume{
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
	m, _ = m.Update(volumeDetailLoadedMsg{volumeID: "vol-1", vol: &volume.Volume{ID: "vol-1", Name: "data"}})
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
	m, _ = m.Update(volumeDetailLoadedMsg{seq: m.refresh.Seq(), volumeID: "vol-1", vol: &volume.Volume{ID: "vol-1", Status: "in-use"}})
	m, _ = m.Update(volumeDetailLoadedMsg{seq: staleSeq, volumeID: "vol-1", vol: &volume.Volume{ID: "vol-1", Status: "available"}})
	m, _ = m.Update(volumeDetailErrMsg{seq: staleSeq, volumeID: "vol-1", err: fmt.Errorf("stale")})
	if m.volume.Status != "in-use" || m.err != "" {
		t.Fatalf("stale response applied: status=%q err=%q", m.volume.Status, m.err)
	}
	if _, next := m.Update(shared.TickMsg{}); next == nil {
		t.Fatal("tick blocked after the newest fetch completed")
	}
}

// A late reply from a previous detail view (whose sequence also started at
// 0) must not be shown for the new volume, or delete would name one volume
// and remove another.
func TestReplyForOtherVolumeIsIgnored(t *testing.T) {
	m := New(nil, nil, "vol-b")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	seq := m.refresh.Seq()

	m, _ = m.Update(volumeDetailLoadedMsg{seq: seq, volumeID: "vol-a", vol: &volume.Volume{ID: "vol-a", Name: "alpha"}})
	m, _ = m.Update(volumeDetailErrMsg{seq: seq, volumeID: "vol-a", err: fmt.Errorf("stale")})
	// A reply tagged with this view's ID but carrying another volume is
	// rejected as well.
	m, _ = m.Update(volumeDetailLoadedMsg{seq: seq, volumeID: "vol-b", vol: &volume.Volume{ID: "vol-a", Name: "alpha"}})
	if m.volume != nil || m.err != "" || !m.loading {
		t.Fatalf("other volume's reply applied: volume=%+v err=%q loading=%v", m.volume, m.err, m.loading)
	}
	if m.SelectedVolumeName() != "vol-b" {
		t.Fatalf("SelectedVolumeName = %q, want vol-b", m.SelectedVolumeName())
	}

	m, _ = m.Update(volumeDetailLoadedMsg{seq: seq, volumeID: "vol-b", vol: &volume.Volume{ID: "vol-b", Name: "bravo"}})
	if m.volume == nil || m.SelectedVolumeName() != "bravo" || m.SelectedVolumeID() != "vol-b" || m.loading {
		t.Fatalf("own reply not applied: volume=%+v loading=%v", m.volume, m.loading)
	}
}

// A server that sends headers and then stops mid-body must not hang the
// fetch: the command's deadline cuts it off and reports an error.
func TestStalledFetchIsCutOffByDeadline(t *testing.T) {
	orig := shared.RequestTimeout
	shared.RequestTimeout = 100 * time.Millisecond
	t.Cleanup(func() { shared.RequestTimeout = orig })

	client, cleanup := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"volume":{"id":"vol-1",`))
		w.(http.Flusher).Flush()
		<-r.Context().Done() // stall until the client gives up
	}))
	defer cleanup()

	m := New(client, nil, "vol-1")
	done := make(chan tea.Msg, 1)
	go func() { done <- m.fetchVolume(0)() }()
	select {
	case msg := <-done:
		if _, ok := msg.(volumeDetailErrMsg); !ok {
			t.Fatalf("got %T, want volumeDetailErrMsg", msg)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("stalled fetch was never cut off")
	}
}
