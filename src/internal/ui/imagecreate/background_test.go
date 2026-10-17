package imagecreate

import (
	"testing"

	"github.com/larkly/lazystack/internal/shared"
)

// A finished upload reports via ResourceActionMsg while its modal is open,
// via UploadFinishedMsg once backgrounded, and never closes a newly opened
// modal that replaced it.
func TestUploadResultRouting(t *testing.T) {
	g := newFakeGlance()
	client := newGlanceClient(t, g, false)

	open, cmd := localModel(client, writeImageFile(t, "data")).submit()
	open, cmd = open.Update(runWork(t, cmd))
	if open.Active || cmd == nil {
		t.Fatal("active upload: modal should close with a result")
	}
	if _, ok := cmd().(shared.ResourceActionMsg); !ok {
		t.Fatal("active upload: want ResourceActionMsg")
	}

	bg, cmd := localModel(client, writeImageFile(t, "data")).submit()
	done := runWork(t, cmd)
	bg.Active = false
	bg, cmd = bg.Update(done)
	if fin, ok := cmd().(UploadFinishedMsg); !ok || fin.Name != "disk" || fin.Err != nil {
		t.Fatalf("backgrounded upload: got %#v", cmd())
	}
	if bg.uploading {
		t.Fatal("backgrounded upload still marked uploading")
	}

	_, cmd = localModel(client, writeImageFile(t, "data")).submit()
	done = runWork(t, cmd)
	reopened := New(client)
	reopened, cmd = reopened.Update(done)
	if !reopened.Active {
		t.Fatal("an earlier upload's result closed the reopened modal")
	}
	if fin, ok := cmd().(UploadFinishedMsg); !ok || fin.Name != "disk" {
		t.Fatalf("stale upload: got %#v", cmd())
	}
}
