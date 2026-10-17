package imagedownload

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Enter on the path field only browses: it opens the directory picker on
// the path, or on the directory holding the file named there, and never
// starts the download. Ctrl+S submits.
func TestEnterOnPathBrowsesAndCtrlSSubmits(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "image.img")
	m := New(nil, "id", "image", "img")
	m.SetSize(100, 30)
	m.pathInput.SetValue(file)
	if h := m.Hints(); !strings.Contains(h, "enter browse") || !strings.Contains(h, "ctrl+s download") {
		t.Fatalf("hints=%q", h)
	}

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.downloading {
		t.Fatal("enter started the download")
	}
	if !m.pickerOpen || m.pickerDir != dir {
		t.Fatalf("enter did not browse the file's directory: open=%v dir=%q", m.pickerOpen, m.pickerDir)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	// A missing parent is reported, not downloaded to.
	m.pathInput.SetValue(filepath.Join(dir, "missing", "image.img"))
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.downloading || m.pickerOpen || !strings.Contains(m.err, "Cannot read directory") {
		t.Fatalf("missing parent: cmd=%v downloading=%v picker=%v err=%q", cmd != nil, m.downloading, m.pickerOpen, m.err)
	}

	m.pathInput.SetValue(file)
	m, cmd = m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil || !m.downloading {
		t.Fatalf("ctrl+s did not submit: err=%q", m.err)
	}
}
