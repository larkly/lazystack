package volumecreate

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
	"github.com/larkly/lazystack/internal/volume"
)

// A successful create used to close the form silently: it must report the
// new volume and hand its ID to the list so the cursor lands on it.
func TestCreatedVolumeIsAnnouncedAndHighlighted(t *testing.T) {
	for _, tc := range []struct{ name, label string }{{"data", "data"}, {"", "v1"}} {
		m := New(nil)
		m.submitting = true
		m, cmd := m.Update(volumeCreatedMsg{vol: &volume.Volume{ID: "v1", Name: tc.name}})
		if m.submitting {
			t.Error("form still submitting")
		}
		got := testutil.CmdMessages(cmd)
		want := []tea.Msg{
			shared.ViewChangeMsg{View: "volumelist", Highlight: "v1"},
			shared.ResourceActionMsg{Action: "Created", Name: tc.label},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("messages = %#v, want %#v", got, want)
		}
	}
}
