package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/imagecreate"
	"github.com/larkly/lazystack/internal/ui/imagedownload"
)

// A cancelled transfer is reported as cancelled, not as a failure; a
// cleanup problem after the cancel is still shown.
func TestCancelledTransferStatus(t *testing.T) {
	m, _ := listFixture(t)
	m.updateImageDownloadBackground(imagedownload.DownloadFinishedMsg{ImageID: "i", Name: "jammy", Err: shared.ErrTransferCanceled})
	if hint := m.statusBar.StickyHint; !strings.Contains(hint, "Image download of jammy cancelled") || strings.Contains(hint, "failed") {
		t.Fatalf("download hint=%q", hint)
	}
	m.updateImageUploadBackground(imagecreate.UploadFinishedMsg{Name: "disk", Err: shared.ErrTransferCanceled})
	if hint := m.statusBar.StickyHint; !strings.Contains(hint, "Image upload of disk cancelled") || strings.Contains(hint, "failed") {
		t.Fatalf("upload hint=%q", hint)
	}
	orphan := fmt.Errorf("%w (cleanup failed, image img-1 may be left behind: boom)", shared.ErrTransferCanceled)
	m.updateImageUploadBackground(imagecreate.UploadFinishedMsg{Name: "disk", Err: orphan})
	if hint := m.statusBar.StickyHint; !strings.Contains(hint, "cancelled") || !strings.Contains(hint, "img-1") {
		t.Fatalf("upload cleanup hint=%q", hint)
	}
}
