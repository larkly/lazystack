package app

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
)

// uploadFixture serves a minimal Glance whose data PUT blocks until release
// is closed, then answers with putStatus.
func uploadFixture(t *testing.T, putStatus int) (Model, chan struct{}) {
	t.Helper()
	release := make(chan struct{})
	var stored int
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/images":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":"img-1","name":"disk","status":"queued"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/images/img-1/file":
			data, _ := io.ReadAll(r.Body)
			<-release
			if putStatus != http.StatusNoContent {
				http.Error(w, "upload refused", putStatus)
				return
			}
			stored = len(data)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/images/img-1":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"id":"img-1","name":"disk","status":"active","size":%d}`, stored)
		case r.Method == http.MethodDelete && r.URL.Path == "/images/img-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	})
	return m, release
}

// startCmds runs every command of a (possibly batched) command tree in its
// own goroutine and delivers the results on the returned channel.
func startCmds(cmd tea.Cmd, out chan<- tea.Msg) {
	if cmd == nil {
		return
	}
	go func() {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				startCmds(c, out)
			}
			return
		}
		out <- msg
	}()
}

// A local upload sent to the background with "b" must still report its
// result in the status bar when it finishes.
func TestBackgroundedImageUploadReportsResult(t *testing.T) {
	for _, tc := range []struct {
		name      string
		putStatus int
		want      string
	}{
		{"success", http.StatusNoContent, shared.Glyph("✓", "OK") + " Image disk.qcow2 uploaded"},
		{"failure", http.StatusInternalServerError, shared.Glyph("✗", "FAIL") + " Image upload failed: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, release := uploadFixture(t, tc.putStatus)
			path := filepath.Join(t.TempDir(), "disk.qcow2")
			if err := os.WriteFile(path, []byte("image-data"), 0o600); err != nil {
				t.Fatal(err)
			}
			m, _ = m.openImageUpload()
			step := func(msg tea.Msg) tea.Cmd {
				next, cmd := m.Update(msg)
				m = next.(Model)
				return cmd
			}
			step(tea.KeyPressMsg{Code: tea.KeyTab})
			for _, r := range path {
				step(tea.KeyPressMsg{Code: r, Text: string(r)})
			}
			results := make(chan tea.Msg, 64)
			startCmds(step(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}), results)
			step(press("b"))
			if m.imageCreate.Active {
				t.Fatal("setup: b should background the upload")
			}
			close(release)

			deadline := time.After(3 * time.Second)
			for m.statusBar.StickyHint == "" {
				select {
				case msg := <-results:
					for _, follow := range quickMessages(step(msg)) {
						step(follow)
					}
				case <-deadline:
					t.Fatalf("no result reported, want prefix %q", tc.want)
				}
			}
			if !strings.HasPrefix(m.statusBar.StickyHint, tc.want) {
				t.Fatalf("hint = %q, want prefix %q", m.statusBar.StickyHint, tc.want)
			}
		})
	}
}
