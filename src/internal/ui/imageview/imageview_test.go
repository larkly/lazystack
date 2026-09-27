package imageview

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/compute"
	img "github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/testutil"
)

func loadedModel(images ...img.Image) Model {
	m := New(nil, nil, time.Minute)
	m.SetSize(120, 40)
	m, _ = m.Update(imagesLoadedMsg{images: images})
	return m
}

func namedImages(names ...string) []img.Image {
	out := make([]img.Image, len(names))
	for i, n := range names {
		out[i] = img.Image{ID: "id-" + n, Name: n, Status: "active"}
	}
	return out
}

// --- #317: usage lookup failures ---

func TestServerUsageErrorIsNotShownAsNoServers(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	client, cleanup := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"servers":[]}`)
	}))
	defer cleanup()

	m := loadedModel(namedImages("alpha", "beta")...)
	m.computeClient = client
	m.cursor = 1

	m, _ = m.Update(m.fetchServers()())
	content := m.renderServersContent(100, 10)
	if strings.Contains(content, "No servers using this image") {
		t.Fatalf("failed lookup rendered as empty usage: %q", content)
	}
	if !strings.Contains(content, "unavailable") {
		t.Fatalf("usage pane = %q, want an unavailable/error state", content)
	}
	if m.ImageID() != "id-beta" {
		t.Fatalf("image cursor changed to %q", m.ImageID())
	}

	// A successful retry clears the error and shows a real empty result.
	fail.Store(false)
	m, _ = m.Update(m.fetchServers()())
	content = m.renderServersContent(100, 10)
	if !strings.Contains(content, "No servers using this image") {
		t.Fatalf("after retry usage pane = %q, want empty result", content)
	}
	if m.ImageID() != "id-beta" || len(m.images) != 2 {
		t.Fatal("retry corrupted image list state")
	}
}

func TestServerUsageErrorHidesStaleServers(t *testing.T) {
	m := loadedModel(namedImages("alpha")...)
	m, _ = m.Update(serversLoadedMsg{servers: []compute.Server{{ID: "s1", Name: "web-1", ImageID: "id-alpha"}}})
	if m.SelectedServerID() != "s1" {
		t.Fatal("expected server usage after successful load")
	}
	m, _ = m.Update(serversLoadedMsg{err: fmt.Errorf("boom")})
	content := m.renderServersContent(100, 10)
	if strings.Contains(content, "web-1") || !strings.Contains(content, "unavailable") {
		t.Fatalf("usage pane = %q, want stale servers hidden behind an error", content)
	}
	if m.SelectedServerID() != "" {
		t.Fatal("stale server still actionable after failed refresh")
	}
}

func TestServerUsageBeforeFirstLoadIsNotEmpty(t *testing.T) {
	m := loadedModel(namedImages("alpha")...)
	if content := m.renderServersContent(100, 10); strings.Contains(content, "No servers using this image") {
		t.Fatalf("usage pane before any load = %q", content)
	}
}

// --- #318: compact server pane scrolling ---

func serversFor(imageID string, n int) []compute.Server {
	out := make([]compute.Server, n)
	for i := range out {
		out[i] = compute.Server{ID: fmt.Sprintf("s%02d", i), Name: fmt.Sprintf("srv-%02d", i), ImageID: imageID, Status: "ACTIVE"}
	}
	return out
}

func assertSelectedVisible(t *testing.T, m Model, maxHeight int, step string) {
	t.Helper()
	want := fmt.Sprintf("\u25b8 srv-%02d", m.serversCursor)
	if content := m.renderServersContent(100, maxHeight); !strings.Contains(content, want) {
		t.Fatalf("%s: cursor %d (scroll %d) not rendered with maxHeight %d:\n%s",
			step, m.serversCursor, m.serversScroll, maxHeight, content)
	}
}

func TestCompactServersKeepCursorVisible(t *testing.T) {
	for _, rows := range []int{1, 2, 3} {
		for _, count := range []int{rows + 1, 2*rows + 1, 2 * rows, 7, 8} {
			for _, height := range []int{12, 20, 30, 60} {
				t.Run(fmt.Sprintf("rows%d/count%d/h%d", rows, count, height), func(t *testing.T) {
					m := loadedModel(namedImages("alpha")...)
					m.SetSize(120, height)
					m, _ = m.Update(serversLoadedMsg{servers: serversFor("id-alpha", count)})
					m.focus = FocusServers
					maxHeight := rows + 1

					assertSelectedVisible(t, m, maxHeight, "start")
					for i := 0; i < count+2; i++ {
						m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
						assertSelectedVisible(t, m, maxHeight, fmt.Sprintf("down %d", i))
					}
					if m.serversCursor != count-1 {
						t.Fatalf("cursor = %d, want last item %d", m.serversCursor, count-1)
					}
					// Resize to full mode and back while at the end.
					assertSelectedVisible(t, m, 30, "full mode")
					assertSelectedVisible(t, m, maxHeight, "compact again")
					for i := 0; i < count+2; i++ {
						m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
						assertSelectedVisible(t, m, maxHeight, fmt.Sprintf("up %d", i))
					}
					if m.serversCursor != 0 {
						t.Fatalf("cursor = %d, want 0", m.serversCursor)
					}
				})
			}
		}
	}
}
