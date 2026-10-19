package imagedownload

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/shared"
)

var (
	keyEsc = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyY   = tea.KeyPressMsg{Code: 'y', Text: "y"}
	keyN   = tea.KeyPressMsg{Code: 'n', Text: "n"}
)

// Esc during a download asks first; n or Esc keep it running, y cancels
// the transfer and closes the modal.
func TestEscAsksBeforeCancellingDownload(t *testing.T) {
	m := New(nil, "img", "image", "img")
	m.SetSize(100, 30)
	m.pathInput.SetValue(filepath.Join(t.TempDir(), "download.img"))
	m, cmd := m.submit()
	if !m.downloading || cmd == nil || m.transfer == nil {
		t.Fatal("submit state")
	}
	if !strings.Contains(m.Hints(), "esc cancel") || !strings.Contains(m.View(), "esc cancel") {
		t.Fatalf("cancel not offered: hints=%q\n%s", m.Hints(), m.View())
	}
	for _, keep := range []tea.KeyPressMsg{keyN, keyEsc} {
		m, _ = m.Update(keyEsc)
		if !m.confirmCancel || !m.Active {
			t.Fatal("esc did not ask for confirmation")
		}
		if !strings.Contains(m.View(), "Cancel the download") || !strings.Contains(m.Hints(), "y cancel") {
			t.Fatalf("prompt missing: hints=%q\n%s", m.Hints(), m.View())
		}
		m, _ = m.Update(keep)
		if m.confirmCancel || !m.Active || !m.downloading || m.transfer.Canceled() {
			t.Fatalf("%s did not keep the download", keep.String())
		}
	}
	m, _ = m.Update(keyEsc)
	m, _ = m.Update(keyY)
	if !m.transfer.Canceled() {
		t.Fatal("y did not call the cancel func")
	}
	if m.Active || m.confirmCancel {
		t.Fatalf("after cancel: active=%v confirming=%v", m.Active, m.confirmCancel)
	}
	m, cmd = m.Update(downloadErrMsg{m.downloadID, "img", shared.ErrTransferCanceled})
	if m.downloading || cmd == nil {
		t.Fatal("cancellation not reported")
	}
	if got := cmd().(DownloadFinishedMsg); !errors.Is(got.Err, shared.ErrTransferCanceled) || got.Name != "image" {
		t.Fatalf("finished=%+v", got)
	}
}

// Cancelling stops a running download and removes the partial file.
func TestCancelStopsDownloadAndRemovesPartialFile(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.Write([]byte(strings.Repeat("x", 4096)))
		w.(http.Flusher).Flush()
		close(started)
		select {
		case <-r.Context().Done():
		case <-release: // test finished without cancelling
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	client := &gophercloud.ServiceClient{ProviderClient: &gophercloud.ProviderClient{}, Endpoint: server.URL + "/"}
	path := filepath.Join(t.TempDir(), "image.img")
	m := New(client, "image-id", "image", "img")
	m.pathInput.SetValue(path)
	m, cmd := m.submit()
	batch := cmd().(tea.BatchMsg)
	result := make(chan tea.Msg, 1)
	go func() { result <- batch[2]() }()

	<-started
	deadline := time.After(5 * time.Second)
	for m.sharedBytesRead.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("no bytes written")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("partial file missing before cancel: %v", err)
	}
	m, _ = m.Update(keyEsc)
	m, _ = m.Update(keyY)

	select {
	case msg := <-result:
		failure, ok := msg.(downloadErrMsg)
		if !ok || !errors.Is(failure.err, shared.ErrTransferCanceled) {
			t.Fatalf("result=%#v", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("download did not stop after cancel")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("partial file remains: %v", err)
	}
}
