package imagecreate

import (
	"errors"
	"net/http"
	"net/http/httptest"
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

// Esc during an upload asks first; n or Esc keep it running, y cancels the
// transfer and closes the modal.
func TestEscAsksBeforeCancellingUpload(t *testing.T) {
	g := newFakeGlance()
	m := localModel(newGlanceClient(t, g, false), writeImageFile(t, "data"))
	m.SetSize(100, 30)
	m, cmd := m.submit()
	if !m.uploading || cmd == nil || m.transfer == nil {
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
		if !strings.Contains(m.View(), "Cancel the upload") || !strings.Contains(m.Hints(), "y cancel") {
			t.Fatalf("prompt missing: hints=%q\n%s", m.Hints(), m.View())
		}
		m, _ = m.Update(keep)
		if m.confirmCancel || !m.Active || !m.uploading || m.transfer.Canceled() {
			t.Fatalf("%s did not keep the upload", keep.String())
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

	// The transfer had not started: the created image is deleted and
	// no data is sent.
	m, cmd = m.Update(runWork(t, cmd))
	if len(g.creates) != 1 || len(g.putBodies) != 0 || len(g.deletes) != 1 {
		t.Fatalf("creates=%d puts=%d deletes=%v", len(g.creates), len(g.putBodies), g.deletes)
	}
	if m.uploading || cmd == nil {
		t.Fatal("cancellation not reported")
	}
	if got := cmd().(UploadFinishedMsg); !errors.Is(got.Err, shared.ErrTransferCanceled) || got.Name != "disk" {
		t.Fatalf("finished=%+v", got)
	}
}

// blockingPut stalls the data upload until the client gives up, so a test
// can cancel it mid-transfer; everything else goes to the fake Glance.
type blockingPut struct {
	*fakeGlance
	started chan struct{}
	release chan struct{} // closed when the test ends
}

func (b blockingPut) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		buf := make([]byte, 1)
		r.Body.Read(buf)
		close(b.started)
		select {
		case <-r.Context().Done():
		case <-b.release:
		}
		return
	}
	b.fakeGlance.ServeHTTP(w, r)
}

// Cancelling a running upload stops it and deletes the image it created.
func TestCancelStopsUploadAndDeletesImage(t *testing.T) {
	g := newFakeGlance()
	h := blockingPut{fakeGlance: g, started: make(chan struct{}), release: make(chan struct{})}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(h.release) })
	client := &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{HTTPClient: *srv.Client()},
		Endpoint:       srv.URL + "/",
	}
	m := localModel(client, writeImageFile(t, strings.Repeat("d", 1<<20)))
	m, cmd := m.submit()
	result := make(chan tea.Msg, 1)
	go func() { result <- runWork(t, cmd) }()

	select {
	case <-h.started:
	case <-time.After(5 * time.Second):
		t.Fatal("upload did not start")
	}
	m, _ = m.Update(keyEsc)
	_, _ = m.Update(keyY)

	select {
	case msg := <-result:
		failure, ok := msg.(uploadErrMsg)
		if !ok || !errors.Is(failure.err, shared.ErrTransferCanceled) {
			t.Fatalf("result=%#v", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("upload did not stop after cancel")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.deletes) != 1 {
		t.Fatalf("deletes=%v, want the created image removed", g.deletes)
	}
}
