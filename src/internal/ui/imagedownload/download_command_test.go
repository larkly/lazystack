package imagedownload

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
)

func TestDownloadCommandWritesPrivateFileAndHandlesFailures(t *testing.T) {
	for _, scenario := range []string{"success", "http-error", "truncated", "missing-parent", "overwrite-race"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/images/image-id/file" {
					t.Errorf("request=%s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if scenario == "http-error" {
					http.Error(w, "denied", http.StatusForbidden)
					return
				}
				if scenario == "truncated" {
					w.Header().Set("Content-Length", "100")
				} else {
					w.Header().Set("Content-Length", "7")
				}
				fmt.Fprint(w, "payload")
			}))
			defer server.Close()
			client := &gophercloud.ServiceClient{ProviderClient: &gophercloud.ProviderClient{}, Endpoint: server.URL + "/"}
			path := filepath.Join(t.TempDir(), "image.img")
			if scenario == "missing-parent" {
				path = filepath.Join(t.TempDir(), "missing", "image.img")
			}
			m := New(client, "image-id", "image", "img")
			m.pathInput.SetValue(path)
			m, cmd := m.submit()
			if cmd == nil {
				t.Fatal("no submit command")
			}
			if scenario == "overwrite-race" {
				if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			// Execute only the work command: spinner/timer commands belong to Tea's event loop.
			batch, ok := cmd().(tea.BatchMsg)
			if !ok || len(batch) != 3 {
				t.Fatalf("submit batch=%T", batch)
			}
			result := batch[2]()
			if scenario == "success" {
				done, ok := result.(downloadDoneMsg)
				if !ok || done.downloadID != m.downloadID || done.imageID != "image-id" || done.name != "image" {
					t.Fatalf("result=%+v", result)
				}
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "payload" {
					t.Fatalf("file=%q err=%v", data, err)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0600 {
					t.Fatalf("mode=%o", info.Mode().Perm())
				}
				if m.sharedBytesRead.Load() != 7 || m.sharedTotal.Load() != 7 {
					t.Fatal("progress counters")
				}
			} else {
				failure, ok := result.(downloadErrMsg)
				if !ok || failure.downloadID != m.downloadID || failure.imageID != "image-id" || failure.err == nil {
					t.Fatalf("result=%+v", result)
				}
				if scenario == "overwrite-race" {
					data, err := os.ReadFile(path)
					if err != nil || string(data) != "original" {
						t.Fatal("existing file overwritten")
					}
				} else if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("partial file remains: %v", err)
				}
				if scenario == "truncated" && !strings.Contains(failure.err.Error(), "writing file") {
					t.Fatal(failure.err)
				}
			}
		})
	}
}
