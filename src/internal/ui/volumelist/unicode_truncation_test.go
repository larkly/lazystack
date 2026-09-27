package volumelist

import (
	"testing"

	"github.com/larkly/lazystack/internal/testutil"
	"github.com/larkly/lazystack/internal/volume"
)

func TestVolumeListTruncatesUnicodeSafely(t *testing.T) {
	for _, width := range []int{30, 50, 79, 100, 140} {
		m := New(nil, nil, 0)
		m.SetSize(width, 40)
		m, _ = m.Update(volumesLoadedMsg{volumes: []volume.Volume{
			{ID: "v", Name: testutil.UnicodeName, Status: "available", Size: 10, VolumeType: testutil.UnicodeName},
			{ID: "abcdefg", Status: "in-use", Size: 1},
			{ID: "", Status: "error"},
		}})
		testutil.AssertRenderSafe(t, m.View(), width)
	}
}
