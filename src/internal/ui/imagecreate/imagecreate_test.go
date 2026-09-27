package imagecreate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
)

// fakeGlance is a minimal Glance endpoint recording the calls lazystack makes
// while creating an image and uploading or importing its data.
type fakeGlance struct {
	mu sync.Mutex

	createStatus int
	putStatuses  []int // per-attempt PUT status; the last entry repeats
	importStatus int
	deleteStatus int
	// reportedSize overrides the size returned by GET after upload; -1 means
	// "report the bytes actually received".
	reportedSize int64

	creates    []map[string]any
	putBodies  [][]byte
	importURIs []string
	deletes    []string
	gets       int
}

func newFakeGlance() *fakeGlance {
	return &fakeGlance{
		createStatus: http.StatusCreated,
		putStatuses:  []int{http.StatusNoContent},
		importStatus: http.StatusAccepted,
		deleteStatus: http.StatusNoContent,
		reportedSize: -1,
	}
}

func (g *fakeGlance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/images":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		g.creates = append(g.creates, body)
		if g.createStatus != http.StatusCreated {
			http.Error(w, "create refused", g.createStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":"img-orphan-1","name":"x","status":"queued"}`)
	case r.Method == http.MethodPut && r.URL.Path == "/images/img-orphan-1/file":
		data, _ := io.ReadAll(r.Body)
		g.putBodies = append(g.putBodies, data)
		idx := len(g.putBodies) - 1
		if idx >= len(g.putStatuses) {
			idx = len(g.putStatuses) - 1
		}
		status := g.putStatuses[idx]
		if status != http.StatusNoContent {
			http.Error(w, "upload refused", status)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && r.URL.Path == "/images/img-orphan-1":
		g.gets++
		size := g.reportedSize
		if size < 0 && len(g.putBodies) > 0 {
			size = int64(len(g.putBodies[len(g.putBodies)-1]))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"img-orphan-1","name":"x","status":"active","size":%d}`, size)
	case r.Method == http.MethodPost && r.URL.Path == "/images/img-orphan-1/import":
		var body struct {
			Method struct {
				URI string `json:"uri"`
			} `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		g.importURIs = append(g.importURIs, body.Method.URI)
		if g.importStatus != http.StatusAccepted {
			http.Error(w, "import refused", g.importStatus)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	case r.Method == http.MethodDelete && r.URL.Path == "/images/img-orphan-1":
		g.deletes = append(g.deletes, "img-orphan-1")
		if g.deleteStatus != http.StatusNoContent {
			http.Error(w, "delete refused", g.deleteStatus)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}
}

func newGlanceClient(t *testing.T, g *fakeGlance, reauth bool) *gophercloud.ServiceClient {
	t.Helper()
	srv := httptest.NewServer(g)
	t.Cleanup(srv.Close)
	pc := &gophercloud.ProviderClient{HTTPClient: *srv.Client()}
	if reauth {
		pc.ReauthFunc = func(context.Context) error { return nil }
	}
	return &gophercloud.ServiceClient{ProviderClient: pc, Endpoint: srv.URL + "/"}
}

// runWork executes only the work command of a submit batch; spinner and
// progress ticks belong to Tea's event loop.
func runWork(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a submit command")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("submit returned %T, want tea.BatchMsg", batch)
	}
	return batch[len(batch)-1]()
}

func writeImageFile(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "disk.qcow2")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func localModel(client *gophercloud.ServiceClient, path string) Model {
	m := New(client)
	m.nameInput.SetValue("disk")
	m.pathInput.SetValue(path)
	return m
}

func urlModel(client *gophercloud.ServiceClient, url string) Model {
	m := New(client)
	m.source = 1
	m.nameInput.SetValue("remote")
	m.pathInput.SetValue(url)
	return m
}
