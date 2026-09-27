package imagedownload

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

// A download whose server stops sending mid-body is abandoned once it
// makes no progress, instead of hanging forever.
func TestStalledDownloadIsCutOff(t *testing.T) {
	orig := shared.TransferStallTimeout
	shared.TransferStallTimeout = 100 * time.Millisecond
	t.Cleanup(func() { shared.TransferStallTimeout = orig })

	client, cleanup := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		<-r.Context().Done() // stall until the client gives up
	}))
	defer cleanup()

	path := filepath.Join(t.TempDir(), "out.img")
	m := New(client, "img-1", "img", "")
	m.pathInput.SetValue(path)
	m, cmd := m.submit()
	if !m.downloading {
		t.Fatal("download did not start")
	}

	got := make(chan tea.Msg, 8)
	for _, c := range cmd().(tea.BatchMsg) {
		go func(c tea.Cmd) { got <- c() }(c)
	}
	deadline := time.After(10 * time.Second)
	for {
		select {
		case msg := <-got:
			e, ok := msg.(downloadErrMsg)
			if !ok {
				continue
			}
			if !strings.Contains(e.err.Error(), "no progress") {
				t.Fatalf("err = %v, want a stall error", e.err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("partial file left behind: %v", err)
			}
			return
		case <-deadline:
			t.Fatal("stalled download was never cut off")
		}
	}
}
