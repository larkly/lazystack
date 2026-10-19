package lblistenercreate

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/larkly/lazystack/internal/testutil"
)

func editListener(t *testing.T, f *fakeListeners, connLimit int) Model {
	t.Helper()
	client, cleanup := testutil.FakeServiceClient(f)
	t.Cleanup(cleanup)
	return NewEdit(client, "lst-1", "front", "", connLimit, "edge", "HTTP", "", lbPools)
}

// Clearing the connection limit in the edit form makes it unlimited (-1)
// instead of silently keeping the old limit.
func TestEditClearedConnLimitSendsUnlimited(t *testing.T) {
	f := &fakeListeners{}
	m := editListener(t, f, 100)
	m.connLimitInput.SetValue("")
	m, msg := runSubmit(t, m)
	if _, ok := msg.(listenerCreatedMsg); !ok {
		t.Fatalf("result = %#v, want success (err %q)", msg, m.err)
	}
	if len(f.bodies) != 1 || f.bodies[0]["connection_limit"] != float64(-1) {
		t.Fatalf("PUT bodies = %v, want connection_limit -1", f.bodies)
	}
}

func TestEditKeptConnLimitIsSent(t *testing.T) {
	f := &fakeListeners{}
	m := editListener(t, f, 100)
	runSubmit(t, m)
	if len(f.bodies) != 1 || f.bodies[0]["connection_limit"] != float64(100) {
		t.Fatalf("PUT bodies = %v, want connection_limit 100", f.bodies)
	}
}

// The placeholder, seen once the field is cleared, says empty means
// unlimited and shows the current value.
func TestEditConnLimitPlaceholderShowsCurrentValue(t *testing.T) {
	m := NewEdit(nil, "lst-1", "front", "", 100, "edge", "HTTP", "", lbPools)
	m.connLimitInput.SetValue("")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "unlimited (current: 100)") {
		t.Fatalf("placeholder not shown in full:\n%s", view)
	}
	m = NewEdit(nil, "lst-1", "front", "", -1, "edge", "HTTP", "", lbPools)
	if got := m.connLimitInput.Placeholder; got != "unlimited" {
		t.Fatalf("placeholder for an unlimited listener = %q, want unlimited", got)
	}
}

func TestConnLimitRejectsNonPositive(t *testing.T) {
	for _, v := range []string{"0", "-1", "abc"} {
		m := NewEdit(nil, "lst-1", "front", "", 100, "edge", "HTTP", "", lbPools)
		m.connLimitInput.SetValue(v)
		m, cmd := m.submit()
		if cmd != nil || !strings.Contains(m.err, "Connection limit") {
			t.Errorf("%q: cmd=%v err=%q, want rejection", v, cmd != nil, m.err)
		}
	}
}

func TestCreateRejectsOutOfRangePort(t *testing.T) {
	for _, v := range []string{"0", "65536", "http"} {
		m := New(nil, "lb-1", "edge", lbPools)
		m.nameInput.SetValue("front")
		m.portInput.SetValue(v)
		m, cmd := m.submit()
		if cmd != nil || !strings.Contains(m.err, "between 1 and 65535") {
			t.Errorf("%q: cmd=%v err=%q, want port rejection", v, cmd != nil, m.err)
		}
	}
}
