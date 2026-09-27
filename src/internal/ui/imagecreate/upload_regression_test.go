package imagecreate

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

// #267: a 401 after the body was consumed must replay the full file.
func TestUploadReplaysFullBodyAfterReauth(t *testing.T) {
	g := newFakeGlance()
	g.putStatuses = []int{http.StatusUnauthorized, http.StatusNoContent}
	client := newGlanceClient(t, g, true)
	m := localModel(client, writeImageFile(t, "image-data"))

	_, cmd := m.submit()
	msg := runWork(t, cmd)

	if _, ok := msg.(uploadDoneMsg); !ok {
		t.Fatalf("result = %#v, want uploadDoneMsg", msg)
	}
	if len(g.putBodies) != 2 {
		t.Fatalf("PUT attempts = %d, want 2", len(g.putBodies))
	}
	for i, body := range g.putBodies {
		if string(body) != "image-data" {
			t.Fatalf("PUT attempt %d body = %q, want full file", i+1, body)
		}
	}
}

// #267: a file that shrinks between stat and upload must not create a
// truncated image; the upload fails and the image is deleted.
func TestUploadFailsWhenFileShrinks(t *testing.T) {
	g := newFakeGlance()
	client := newGlanceClient(t, g, false)
	path := writeImageFile(t, "0123456789")
	m := localModel(client, path)

	_, cmd := m.submit()
	if err := os.WriteFile(path, []byte("01234"), 0o600); err != nil {
		t.Fatal(err)
	}
	msg := runWork(t, cmd)

	if _, ok := msg.(uploadErrMsg); !ok {
		t.Fatalf("result = %#v, want uploadErrMsg", msg)
	}
	if len(g.deletes) != 1 {
		t.Fatalf("deletes = %v, want cleanup of the created image", g.deletes)
	}
}

// #267: a file that grows between stat and upload fails as well.
func TestUploadFailsWhenFileGrows(t *testing.T) {
	g := newFakeGlance()
	client := newGlanceClient(t, g, false)
	path := writeImageFile(t, "01234")
	m := localModel(client, path)

	_, cmd := m.submit()
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	msg := runWork(t, cmd)

	if _, ok := msg.(uploadErrMsg); !ok {
		t.Fatalf("result = %#v, want uploadErrMsg", msg)
	}
	if len(g.deletes) != 1 {
		t.Fatalf("deletes = %v, want cleanup of the created image", g.deletes)
	}
}

// #267: Glance reporting a different stored size than what was sent is a
// failed upload, not a success.
func TestUploadFailsWhenStoredSizeDiffers(t *testing.T) {
	g := newFakeGlance()
	g.reportedSize = 3
	client := newGlanceClient(t, g, false)
	m := localModel(client, writeImageFile(t, "image-data"))

	_, cmd := m.submit()
	msg := runWork(t, cmd)

	errMsg, ok := msg.(uploadErrMsg)
	if !ok {
		t.Fatalf("result = %#v, want uploadErrMsg", msg)
	}
	if !strings.Contains(errMsg.err.Error(), "size") {
		t.Fatalf("error = %v, want size mismatch", errMsg.err)
	}
	if len(g.deletes) != 1 {
		t.Fatalf("deletes = %v, want cleanup", g.deletes)
	}
}

// #267/#269: progress reflects the uploaded bytes through the shared reader.
func TestUploadProgressTracksBytes(t *testing.T) {
	g := newFakeGlance()
	client := newGlanceClient(t, g, false)
	m := localModel(client, writeImageFile(t, "image-data"))

	m, cmd := m.submit()
	if _, ok := runWork(t, cmd).(uploadDoneMsg); !ok {
		t.Fatal("upload should succeed")
	}
	m, _ = m.Update(progressTickMsg{})
	if m.bytesRead != 10 || m.totalBytes != 10 {
		t.Fatalf("progress = %d/%d, want 10/10", m.bytesRead, m.totalBytes)
	}
}

// #314: upload failure plus cleanup failure keeps the primary error and names
// the orphaned image.
func TestUploadFailureReportsOrphanWhenCleanupFails(t *testing.T) {
	g := newFakeGlance()
	g.putStatuses = []int{http.StatusInternalServerError}
	g.deleteStatus = http.StatusInternalServerError
	client := newGlanceClient(t, g, false)
	m := localModel(client, writeImageFile(t, "image-data"))

	_, cmd := m.submit()
	msg := runWork(t, cmd)

	errMsg, ok := msg.(uploadErrMsg)
	if !ok {
		t.Fatalf("result = %#v, want uploadErrMsg", msg)
	}
	text := errMsg.err.Error()
	if !strings.Contains(text, "uploading image data") {
		t.Fatalf("error %q lost the primary upload failure", text)
	}
	if !strings.Contains(text, "img-orphan-1") || !strings.Contains(text, "cleanup") {
		t.Fatalf("error %q does not name the orphaned image and cleanup failure", text)
	}
}

// #314: a successful cleanup keeps the primary error unchanged.
func TestUploadFailureWithSuccessfulCleanupKeepsPrimaryError(t *testing.T) {
	g := newFakeGlance()
	g.putStatuses = []int{http.StatusInternalServerError}
	client := newGlanceClient(t, g, false)
	m := localModel(client, writeImageFile(t, "image-data"))

	_, cmd := m.submit()
	errMsg, ok := runWork(t, cmd).(uploadErrMsg)
	if !ok {
		t.Fatal("want uploadErrMsg")
	}
	if strings.Contains(errMsg.err.Error(), "cleanup") {
		t.Fatalf("error %q mentions cleanup although it succeeded", errMsg.err)
	}
	if len(g.deletes) != 1 {
		t.Fatalf("deletes = %v, want one cleanup", g.deletes)
	}
}

// #314: no cleanup when the image itself could not be created.
func TestNoCleanupWhenCreateFails(t *testing.T) {
	g := newFakeGlance()
	g.createStatus = http.StatusInternalServerError
	client := newGlanceClient(t, g, false)
	m := localModel(client, writeImageFile(t, "image-data"))

	_, cmd := m.submit()
	if _, ok := runWork(t, cmd).(uploadErrMsg); !ok {
		t.Fatal("want uploadErrMsg")
	}
	if len(g.deletes) != 0 || len(g.putBodies) != 0 {
		t.Fatalf("deletes=%v puts=%d, want none", g.deletes, len(g.putBodies))
	}
}

// #314: import failure plus cleanup failure names the orphaned image.
func TestImportFailureReportsOrphanWhenCleanupFails(t *testing.T) {
	g := newFakeGlance()
	g.importStatus = http.StatusBadRequest
	g.deleteStatus = http.StatusInternalServerError
	client := newGlanceClient(t, g, false)
	m := urlModel(client, "https://example.com/img.qcow2")

	_, cmd := m.submit()
	errMsg, ok := runWork(t, cmd).(uploadErrMsg)
	if !ok {
		t.Fatal("want uploadErrMsg")
	}
	text := errMsg.err.Error()
	if !strings.Contains(text, "importing image") || !strings.Contains(text, "img-orphan-1") {
		t.Fatalf("error %q should keep the import failure and name the orphan", text)
	}
}

// #324: malformed or unsupported URLs are rejected before any Glance call.
func TestURLImportRejectsInvalidURLsBeforeCreate(t *testing.T) {
	for _, raw := range []string{
		"not a url",
		"example.com/image.qcow2",
		"https://",
		"https:///image.qcow2",
		"ftp://example.com/image.qcow2",
		"file:///etc/passwd",
		"http//example.com/x.qcow2",
	} {
		t.Run(raw, func(t *testing.T) {
			g := newFakeGlance()
			client := newGlanceClient(t, g, false)
			m := urlModel(client, raw)

			m, cmd := m.submit()
			if cmd != nil {
				t.Fatalf("submit returned a command for %q", raw)
			}
			if m.err == "" {
				t.Fatalf("no inline error for %q", raw)
			}
			if len(g.creates) != 0 {
				t.Fatal("image was created before URL validation")
			}
		})
	}
}

// #324: valid URLs are passed through exactly, including the query string.
func TestURLImportPreservesURLExactly(t *testing.T) {
	const raw = "https://images.example.com/cloud/jammy.qcow2?sig=a%2Fb&exp=1"
	g := newFakeGlance()
	client := newGlanceClient(t, g, false)
	m := urlModel(client, raw)

	_, cmd := m.submit()
	if _, ok := runWork(t, cmd).(importStartedMsg); !ok {
		t.Fatal("want importStartedMsg")
	}
	if len(g.importURIs) != 1 || g.importURIs[0] != raw {
		t.Fatalf("import URIs = %v, want [%s]", g.importURIs, raw)
	}
}

// #324: the default name for a URL comes from the URL path, not the query.
func TestURLImportDerivesNameFromURLPath(t *testing.T) {
	g := newFakeGlance()
	client := newGlanceClient(t, g, false)
	m := urlModel(client, "https://example.com/dl/jammy%20server.qcow2?token=a/b.img")
	m.nameInput.SetValue("")

	m, cmd := m.submit()
	if cmd == nil {
		t.Fatalf("submit rejected: %s", m.err)
	}
	if got := m.nameInput.Value(); got != "jammy server.qcow2" {
		t.Fatalf("derived name = %q, want %q", got, "jammy server.qcow2")
	}
}

// #325: invalid explicit min disk/RAM values fail inline before any API call.
// Overflow is covered by image.ParseMinimum; the inputs' character limits
// keep such values out of the form.
func TestSubmitRejectsInvalidMinimums(t *testing.T) {
	for _, tc := range []struct{ disk, ram string }{
		{"abc", ""},
		{"-1", ""},
		{"1.5", ""},
		{"", "abc"},
		{"", "-5"},
		{"", "1e3"},
	} {
		for _, source := range []int{0, 1} {
			t.Run(fmt.Sprintf("src%d/%s/%s", source, tc.disk, tc.ram), func(t *testing.T) {
				g := newFakeGlance()
				client := newGlanceClient(t, g, false)
				var m Model
				if source == 0 {
					m = localModel(client, writeImageFile(t, "image-data"))
				} else {
					m = urlModel(client, "https://example.com/img.qcow2")
				}
				m.minDiskInput.SetValue(tc.disk)
				m.minRAMInput.SetValue(tc.ram)

				m, cmd := m.submit()
				if cmd != nil || m.err == "" {
					t.Fatalf("cmd=%v err=%q, want inline validation error", cmd != nil, m.err)
				}
				if len(g.creates) != 0 {
					t.Fatal("image created despite invalid minimums")
				}
			})
		}
	}
}

// #325: blank uses the default 0 and valid values are sent exactly.
func TestSubmitSendsValidMinimums(t *testing.T) {
	for _, source := range []int{0, 1} {
		for _, tc := range []struct {
			disk, ram         string
			wantDisk, wantRAM float64
		}{
			{"", "", 0, 0},
			{"20", "2048", 20, 2048},
			{" 0 ", "0", 0, 0},
		} {
			t.Run(fmt.Sprintf("src%d/%s/%s", source, tc.disk, tc.ram), func(t *testing.T) {
				g := newFakeGlance()
				client := newGlanceClient(t, g, false)
				var m Model
				if source == 0 {
					m = localModel(client, writeImageFile(t, "image-data"))
				} else {
					m = urlModel(client, "https://example.com/img.qcow2")
				}
				m.minDiskInput.SetValue(tc.disk)
				m.minRAMInput.SetValue(tc.ram)

				m, cmd := m.submit()
				if cmd == nil {
					t.Fatalf("submit rejected: %s", m.err)
				}
				runWork(t, cmd)
				if len(g.creates) != 1 {
					t.Fatalf("creates = %d, want 1", len(g.creates))
				}
				body := g.creates[0]
				gotDisk, _ := body["min_disk"].(float64)
				gotRAM, _ := body["min_ram"].(float64)
				if gotDisk != tc.wantDisk || gotRAM != tc.wantRAM {
					t.Fatalf("min_disk=%v min_ram=%v, want %v/%v", body["min_disk"], body["min_ram"], tc.wantDisk, tc.wantRAM)
				}
			})
		}
	}
}
