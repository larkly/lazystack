package imagecreate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Confirming the large-file prompt after the file disappeared reports an
// error instead of crashing.
func TestLargeFileConfirmAfterFileRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.raw")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Sparse file just over the 10 GB warning threshold.
	if err := f.Truncate(10*1024*1024*1024 + 1); err != nil {
		f.Close()
		t.Skipf("cannot create sparse file: %v", err)
	}
	f.Close()

	m := localModel(nil, path)
	m, cmd := m.submit()
	if cmd != nil || !m.warnLargeFile {
		t.Fatalf("expected the large-file prompt, got cmd=%v warn=%v err=%q", cmd != nil, m.warnLargeFile, m.err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	m, cmd = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd != nil || m.uploading || m.submitting {
		t.Fatalf("upload started for a missing file (uploading=%v)", m.uploading)
	}
	if !strings.Contains(m.err, "not found") {
		t.Fatalf("err = %q, want a file-not-found error", m.err)
	}
}
