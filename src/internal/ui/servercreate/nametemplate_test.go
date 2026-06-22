package servercreate

import (
	"regexp"
	"strings"
	"testing"
)

func TestExpandNameTemplate(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		idx  int
		want string
	}{
		{"plain index", "web-{n}", 0, "web-0"},
		{"padded index", "web-{n:02d}", 5, "web-05"},
		{"padded index wide", "web-{n:04d}", 5, "web-0005"},
		{"multiple placeholders", "{n}-{random}-{timestamp}", 3, "3-[a-f0-9]{6}-[0-9]+"},
		{"no placeholders", "static-name", 0, "static-name"},
		{"invalid width", "web-{n:abc}", 0, "web-{n:abc}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandNameTemplate(tt.tmpl, tt.idx)
			if tt.want != "" && strings.ContainsAny(tt.want, "[](){}+*?^$.\\|") {
				re := regexp.MustCompile("^" + tt.want + "$")
				if !re.MatchString(got) {
					t.Errorf("expandNameTemplate(%q, %d) = %q; want match %q", tt.tmpl, tt.idx, got, tt.want)
				}
				return
			}
			if got != tt.want {
				t.Errorf("expandNameTemplate(%q, %d) = %q; want %q", tt.tmpl, tt.idx, got, tt.want)
			}
		})
	}
}

func TestPreviewNames(t *testing.T) {
	out := previewNames("web-{n:02d}", 5, 3)
	if len(out) != 3 {
		t.Fatalf("expected 3 previews, got %d", len(out))
	}
	if out[0] != "web-00" || out[1] != "web-01" || out[2] != "web-02" {
		t.Errorf("unexpected previews: %v", out)
	}
}

func TestPreviewNamesEmptyTemplate(t *testing.T) {
	out := previewNames("", 5, 3)
	if len(out) != 0 {
		t.Errorf("expected empty preview for empty template, got %v", out)
	}
}

func TestPreviewNamesCountOne(t *testing.T) {
	out := previewNames("web-{n:02d}", 1, 3)
	if len(out) != 0 {
		t.Errorf("expected empty preview for count=1, got %v", out)
	}
}
