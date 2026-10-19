package shared

import "testing"

func TestPadContent(t *testing.T) {
	if got := PadContent("Title", ""); got != " Title\n" {
		t.Fatalf("empty content = %q", got)
	}
	if got := PadContent("T", "a\nb"); got != " T\n\n a\n b" {
		t.Fatalf("content = %q", got)
	}
}
