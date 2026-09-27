package imagedownload

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// #326: generated default names are safe single path components.
func TestDefaultFilenameIsSafe(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, format, want string }{
		{"my/image name", "", "my_image_name.img"},
		{"Ubuntu 日本語", "qcow2", "Ubuntu_日本語.qcow2"},
		{"a\x00b\nc\td\x1b[31m", "raw", "a_b_c_d_[31m.raw"},
		{`win:dows*na?me"<x>|y\z`, "iso", "win_dows_na_me__x__y_z.iso"},
		{"", "qcow2", "image.qcow2"},
		{".", "qcow2", "image.qcow2"},
		{"..", "qcow2", "image.qcow2"},
		{"../../etc/passwd", "raw", "_.._etc_passwd.raw"},
		{".hidden", "raw", "hidden.raw"},
		{"image", "../x", "image.img"},
		{"bad\xffutf8", "raw", "bad_utf8.raw"},
	} {
		got := defaultFilename(tc.name, tc.format)
		if got != tc.want {
			t.Errorf("defaultFilename(%q, %q) = %q, want %q", tc.name, tc.format, got, tc.want)
		}
		if filepath.Dir(filepath.Join(dir, got)) != dir {
			t.Errorf("defaultFilename(%q) = %q escapes the directory", tc.name, got)
		}
		for _, r := range got {
			if r < 0x20 || r == 0x7f {
				t.Errorf("defaultFilename(%q) = %q contains control characters", tc.name, got)
			}
		}
	}
	long := defaultFilename(strings.Repeat("é", 300), "qcow2")
	if len(long) > 255 || !strings.HasSuffix(long, ".qcow2") || !utf8.ValidString(long) {
		t.Errorf("long name = %d bytes %q, want <= 255 bytes of valid UTF-8 ending in .qcow2", len(long), long)
	}
}

// #326: New uses the safe default name.
func TestNewUsesSafeDefaultFilename(t *testing.T) {
	m := New(nil, "id", "evil\nname\x00", "raw")
	if m.defaultFile != "evil_name_.raw" || filepath.Base(m.pathInput.Value()) != m.defaultFile {
		t.Fatalf("defaultFile = %q path = %q", m.defaultFile, m.pathInput.Value())
	}
}

// #326: the user's chosen path is used as typed, not re-sanitized.
func TestUserPathIsNotSanitized(t *testing.T) {
	dir := t.TempDir()
	chosen := filepath.Join(dir, "my image (v2).qcow2")
	m := New(nil, "id", "image", "qcow2")
	m.pathInput.SetValue(chosen)
	m, cmd := m.submit()
	if cmd == nil || m.err != "" {
		t.Fatalf("submit rejected %q: %s", chosen, m.err)
	}
	if m.pathInput.Value() != chosen {
		t.Fatalf("path changed to %q", m.pathInput.Value())
	}
}
