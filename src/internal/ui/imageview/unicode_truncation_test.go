package imageview

import (
	"testing"

	img "github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/testutil"
)

func TestImageViewTruncatesUnicodeSafely(t *testing.T) {
	for _, width := range []int{30, 50, 79, 100, 140} {
		m := New(nil, nil, 0)
		m.SetSize(width, 40)
		m, _ = m.Update(imagesLoadedMsg{images: []img.Image{
			{ID: "i", Name: testutil.UnicodeName, Status: "active", DiskFormat: "qcow2", Visibility: "private", Tags: []string{testutil.UnicodeName}},
			{ID: "", Status: "queued"},
			{ID: "abcdefg", Status: "active"},
		}})
		testutil.AssertRenderSafe(t, m.View(), width)
	}
}
