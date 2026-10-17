package keypaircreate

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

// A successful import used to close the form silently: it must report the
// new key pair and hand its name to the list so the cursor lands on it.
func TestImportedKeypairIsAnnouncedAndHighlighted(t *testing.T) {
	m := New(nil)
	m, cmd := m.Update(keypairCreatedMsg{kp: &compute.KeyPairFull{Name: "deploy"}})
	got := testutil.CmdMessages(cmd)
	want := []tea.Msg{
		shared.ViewChangeMsg{View: "keypairlist", Highlight: "deploy"},
		shared.ResourceActionMsg{Action: "Created", Name: "deploy"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %#v, want %#v", got, want)
	}
}

// A generated key pair keeps the form open on its private key: the create
// is reported at once and the list highlights it when the user leaves.
func TestGeneratedKeypairIsAnnouncedThenHighlightedOnLeave(t *testing.T) {
	for _, leave := range []tea.KeyPressMsg{{Code: tea.KeyEnter}, {Code: tea.KeyEscape}} {
		m := New(nil)
		m, cmd := m.Update(keypairCreatedMsg{kp: &compute.KeyPairFull{Name: "gen", PrivateKey: "PRIVATE"}})
		got := testutil.CmdMessages(cmd)
		want := []tea.Msg{shared.ResourceActionMsg{Action: "Created", Name: "gen"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("on create: messages = %#v, want %#v", got, want)
		}
		m, cmd = m.Update(leave)
		got = testutil.CmdMessages(cmd)
		want = []tea.Msg{shared.ViewChangeMsg{View: "keypairlist", Highlight: "gen"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("on %s: messages = %#v, want %#v", leave, got, want)
		}
	}
}
