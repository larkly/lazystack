package imagedownload

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"github.com/larkly/lazystack/internal/shared"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDefaultPathValidationAndCancel(t *testing.T) {
	m := New(nil, "image", "my/image name", "")
	m.SetSize(100, 30)
	if m.defaultFile != "my_image_name.img" || filepath.Base(m.pathInput.Value()) != m.defaultFile || !m.Active || m.Init() == nil {
		t.Fatal("defaults")
	}
	m.pathInput.SetValue(" ")
	m, cmd := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd != nil || m.downloading || !strings.Contains(m.View(), "File path is required") {
		t.Fatal("required path")
	}
	file := filepath.Join(t.TempDir(), "existing")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	m.pathInput.SetValue(file)
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || !strings.Contains(m.err, "already exists") {
		t.Fatal("overwrite validation")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focusField != fieldSubmit || m.pathInput.Focused() {
		t.Fatal("tab focus")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Active {
		t.Fatal("cancel button")
	}
}
func TestDirectoryPicker(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"visible", ".hidden"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "file"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	m := New(nil, "id", "image", "qcow2")
	m.SetSize(100, 30)
	m.pathInput.SetValue(dir)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.pickerOpen || len(m.pickerEntries) != 2 || m.pickerEntries[1].name != "visible/" {
		t.Fatalf("entries=%+v", m.pickerEntries)
	}
	if !strings.Contains(m.View(), "Choose Directory") {
		t.Fatal(m.View())
	}
	for _, code := range []rune{tea.KeyUp, tea.KeyDown, tea.KeyDown} {
		m, _ = m.Update(tea.KeyPressMsg{Code: code})
	}
	if m.pickerCursor != 1 {
		t.Fatal("bounds")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.pickerOpen || m.pathInput.Value() != filepath.Join(dir, "visible", "image.qcow2") {
		t.Fatal("directory selection")
	}
	m, _ = m.openDirPicker(dir)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.pickerOpen || !m.Active {
		t.Fatal("picker cancel should keep modal")
	}
	m, _ = m.openDirPicker(filepath.Join(dir, "missing"))
	if !strings.Contains(m.err, "Cannot read directory") {
		t.Fatal("directory error")
	}
	m.pickerOpen = true
	m.pickerEntries = nil
	m.pickerCursor = 0
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || !strings.Contains(m.View(), "No subdirectories") {
		t.Fatal("empty picker")
	}
}
func TestProgressAndBackgroundCorrelation(t *testing.T) {
	m := New(nil, "current", "image", "img")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.pathInput.SetValue(filepath.Join(t.TempDir(), "download.img"))
	m, cmd := m.submit()
	if !m.downloading || cmd == nil || m.downloadID == 0 {
		t.Fatal("submit state")
	}
	m.sharedBytesRead.Store(50)
	m.sharedTotal.Store(100)
	m, cmd = m.Update(progressTickMsg{})
	if cmd == nil || m.bytesRead != 50 || !strings.Contains(m.View(), "50%") {
		t.Fatal("progress")
	}
	m.bytesRead = 200
	if !strings.Contains(m.View(), "100%") {
		t.Fatal("percent clamp")
	}
	m.totalBytes = 0
	if !strings.Contains(m.View(), "downloaded...") {
		t.Fatal("unknown total")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.Active {
		t.Fatal("escape must not cancel download")
	}
	stale := downloadDoneMsg{downloadID: m.downloadID + 1, imageID: "old", name: "old image"}
	m, cmd = m.Update(stale)
	if !m.downloading || !m.Active {
		t.Fatal("stale completion changed current session")
	}
	if got := cmd().(DownloadFinishedMsg); got.ImageID != "old" || got.Name != "old image" {
		t.Fatalf("stale=%+v", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if m.Active || !m.downloading || !strings.Contains(m.Hints(), "background") {
		t.Fatal("background state")
	}
	m, cmd = m.Update(downloadDoneMsg{m.downloadID, "current", "image"})
	if m.downloading || cmd == nil {
		t.Fatal("background completion")
	}
	if got := cmd().(DownloadFinishedMsg); got.ImageID != "current" || got.Name != "image" {
		t.Fatalf("completion=%+v", got)
	}
}
func TestCompletionErrorsAndCountingReader(t *testing.T) {
	for _, active := range []bool{false, true} {
		for _, stale := range []bool{false, true} {
			m := New(nil, "id", "image", "img")
			m.Active = active
			m.downloading = true
			m.downloadID = 10
			id := int64(10)
			if stale {
				id = 11
			}
			failure := errors.New("download denied")
			m, cmd := m.Update(downloadErrMsg{id, "id", failure})
			if stale {
				if !m.downloading {
					t.Fatal("stale error stopped current download")
				}
			} else if m.downloading || m.err != "download denied" {
				t.Fatal("current error state")
			}
			if !active || stale {
				if cmd == nil {
					t.Fatal("missing notification")
				}
				if got := cmd().(DownloadFinishedMsg); !errors.Is(got.Err, failure) || got.ImageID != "id" {
					t.Fatalf("error notification=%+v", got)
				}
			} else if cmd != nil || !strings.Contains(m.View(), "download denied") {
				t.Fatal("foreground error")
			}
		}
	}
	m := New(nil, "id", "image", "img")
	m.downloadID = 1
	m.downloading = true
	m, cmd := m.Update(downloadDoneMsg{1, "id", "image"})
	if m.Active || m.downloading || cmd == nil {
		t.Fatal("foreground completion")
	}
	if got := cmd().(shared.ResourceActionMsg); got.Action != "Downloaded image" || got.Name != "image" {
		t.Fatalf("completion=%+v", got)
	}
	count := &atomic.Int64{}
	r := &countingReader{strings.NewReader("payload"), count}
	got, err := io.ReadAll(r)
	if err != nil || string(got) != "payload" || count.Load() != 7 {
		t.Fatalf("reader=%q count=%d err=%v", got, count.Load(), err)
	}
}
