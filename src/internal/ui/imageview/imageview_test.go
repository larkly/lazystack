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
	"github.com/larkly/lazystack/internal/shared"
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

	m, _ = m.Update(m.startServersFetch()())
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
	m, _ = m.Update(m.startServersFetch()())
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

// Background ticks must not stack image or server-usage fetches, and slow
// older responses must never overwrite the results of newer fetches.
func TestTickRefreshesDoNotOverlapOrRegress(t *testing.T) {
	client, cleanup := testutil.FakeServiceClient(http.NotFoundHandler())
	defer cleanup()
	m := loadedModel(namedImages("alpha")...)
	m.computeClient = client

	m, first := m.Update(shared.TickMsg{})
	if first == nil {
		t.Fatal("tick did not fetch")
	}
	staleImages, staleServers := m.refresh.Seq(), m.serversRefresh.Seq()
	if !m.serversRefresh.Busy() {
		t.Fatal("tick did not fetch server usage")
	}
	if _, again := m.Update(shared.TickMsg{}); again != nil {
		t.Fatal("tick started a second fetch while one was in flight")
	}

	m.ForceRefresh()
	m, _ = m.Update(imagesLoadedMsg{seq: m.refresh.Seq(), images: namedImages("new")})
	m, _ = m.Update(serversLoadedMsg{seq: m.serversRefresh.Seq(), servers: []compute.Server{{ID: "s-new", ImageID: "id-new"}}})
	m, _ = m.Update(imagesLoadedMsg{seq: staleImages, images: namedImages("old")})
	m, _ = m.Update(imagesErrMsg{seq: staleImages, err: fmt.Errorf("stale")})
	m, _ = m.Update(serversLoadedMsg{seq: staleServers, err: fmt.Errorf("stale")})
	if len(m.images) != 1 || m.images[0].Name != "new" || m.err != "" {
		t.Fatalf("stale image list applied: %v err=%q", m.images, m.err)
	}
	if len(m.servers) != 1 || m.serversErr != "" {
		t.Fatalf("stale server usage applied: %v err=%q", m.servers, m.serversErr)
	}
	if _, next := m.Update(shared.TickMsg{}); next == nil {
		t.Fatal("tick blocked after the newest fetches completed")
	}
}
