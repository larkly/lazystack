package servercreate

import (
	"reflect"
	"testing"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

// A successful create used to close the form silently: it must report the
// new server and hand its ID to the list so the cursor lands on it.
func TestCreatedServerIsAnnouncedAndHighlighted(t *testing.T) {
	cases := []struct {
		name, template, count string
		want                  string
	}{
		{"single", "", "1", "web"},
		{"count", "", "3", "web (3 servers)"},
		{"template", "web-{n:02d}", "3", "web-00, web-01, web-02"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := readyForm(t)
			m.nameInput.SetValue("web")
			m.templateInput.SetValue(tc.template)
			m.countInput.SetValue(tc.count)
			m, cmd := m.submit()
			msgs := run(cmd)
			if len(msgs) != 1 {
				t.Fatalf("create produced %v", msgs)
			}
			m, cmd = m.Update(msgs[0])
			got := testutil.CmdMessages(cmd)
			want := []any{
				shared.ViewChangeMsg{View: "serverlist", Highlight: "srv-1"},
				shared.ResourceActionMsg{Action: "Created", Name: tc.want},
			}
			if !reflect.DeepEqual(toAny(got), want) {
				t.Fatalf("messages = %#v, want %#v", got, want)
			}
		})
	}
}

func toAny[T any](in []T) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}
