package serverresize

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

type resizeRecorder struct {
	mu    sync.Mutex
	posts []string
	fail  bool
}

func (r *resizeRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	if req.Method == http.MethodPost {
		r.posts = append(r.posts, req.URL.Path)
	}
	fail := r.fail
	r.mu.Unlock()
	if fail {
		w.WriteHeader(http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (r *resizeRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.posts)
}

// run executes cmd (expanding batches) and returns every non-spinner message.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, run(c)...)
		}
		return out
	}
	if _, ok := msg.(spinner.TickMsg); ok || msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

var (
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
	keyY     = tea.KeyPressMsg{Code: 'y', Text: "y"}
	keyN     = tea.KeyPressMsg{Code: 'n', Text: "n"}
)

var testFlavors = []compute.Flavor{
	{ID: "f-small", Name: "m1.small", VCPUs: 1, RAM: 2048, Disk: 20},
	{ID: "f-large", Name: "m1.large", VCPUs: 4, RAM: 8192, Disk: 80},
}

func newBulk(t *testing.T) (Model, *resizeRecorder) {
	t.Helper()
	rec := &resizeRecorder{}
	client, cleanup := testutil.FakeServiceClient(rec)
	t.Cleanup(cleanup)
	m := NewBulk(client, []string{"srv-a", "srv-b"}, "m1.small")
	m.SetSize(100, 40)
	m, _ = m.Update(flavorsLoadedMsg{flavors: testFlavors})
	return m, rec
}

func newSingle(t *testing.T) (Model, *resizeRecorder) {
	t.Helper()
	rec := &resizeRecorder{}
	client, cleanup := testutil.FakeServiceClient(rec)
	t.Cleanup(cleanup)
	m := New(client, "srv-a", "web", "m1.small")
	m.SetSize(100, 40)
	m, _ = m.Update(flavorsLoadedMsg{flavors: testFlavors})
	return m, rec
}

func TestBulkResizeRequiresConfirmation(t *testing.T) {
	m, rec := newBulk(t)
	m, _ = m.Update(keyDown)
	m, cmd := m.Update(keyEnter)
	if msgs := run(cmd); len(msgs) != 0 || rec.count() != 0 {
		t.Fatalf("enter executed without confirmation: msgs=%v posts=%d", msgs, rec.count())
	}
	if m.submitting {
		t.Fatal("submitting before confirmation")
	}
	view := m.View()
	if !strings.Contains(view, "2 servers") || !strings.Contains(view, "m1.large") || !strings.Contains(view, "confirm") {
		t.Fatalf("confirmation summary missing:\n%s", view)
	}

	// Cancel returns to the picker without any requests.
	for _, k := range []tea.KeyPressMsg{keyN, keyEsc} {
		m, cmd = m.Update(k)
		run(cmd)
		if !m.Active || m.confirming || rec.count() != 0 {
			t.Fatalf("cancel via %q: active=%v confirming=%v posts=%d", k.String(), m.Active, m.confirming, rec.count())
		}
		m, _ = m.Update(keyEnter)
		if !m.confirming {
			t.Fatal("enter did not reopen confirmation")
		}
	}

	m, cmd = m.Update(keyY)
	if !m.submitting {
		t.Fatal("confirm did not activate the submit guard")
	}
	msgs := run(cmd)
	if rec.count() != 2 {
		t.Fatalf("posts=%d want 2", rec.count())
	}
	if len(msgs) != 1 {
		t.Fatalf("msgs=%v", msgs)
	}
}

func TestSubmittingBlocksRepeatAndEscape(t *testing.T) {
	m, rec := newBulk(t)
	m, _ = m.Update(keyDown)
	m, _ = m.Update(keyEnter)
	m, first := m.Update(keyEnter) // confirm
	if !m.submitting {
		t.Fatal("not submitting after confirm")
	}
	var cmds []tea.Cmd
	for _, k := range []tea.KeyPressMsg{keyEnter, keyEnter, keyY, keyEsc, keyDown, {Code: '/', Text: "/"}} {
		var c tea.Cmd
		m, c = m.Update(k)
		cmds = append(cmds, c)
	}
	if !m.Active || !m.submitting || m.filtering || m.cursor != 1 {
		t.Fatalf("in-flight modal changed: active=%v submitting=%v filtering=%v cursor=%d", m.Active, m.submitting, m.filtering, m.cursor)
	}
	for _, c := range cmds {
		run(c)
	}
	msgs := run(first)
	if rec.count() != 2 {
		t.Fatalf("posts=%d want exactly one per server", rec.count())
	}
	m, done := m.Update(msgs[0])
	if m.Active || m.submitting {
		t.Fatal("success did not close")
	}
	out := run(done)
	if len(out) != 1 {
		t.Fatalf("completion msgs=%v", out)
	}
	if got, ok := out[0].(shared.ServerActionMsg); !ok || got.Action != "Resize" {
		t.Fatalf("completion=%#v", out[0])
	}
}

func TestResizeErrorUnlocksAndRetries(t *testing.T) {
	m, rec := newSingle(t)
	m, _ = m.Update(keyDown)
	m, cmd := m.Update(keyEnter)
	if !m.submitting {
		t.Fatal("single resize should submit directly")
	}
	m, again := m.Update(keyEnter)
	if again != nil {
		t.Fatal("second enter while submitting produced a command")
	}
	rec.fail = true
	msgs := run(cmd)
	if rec.count() != 1 {
		t.Fatalf("posts=%d", rec.count())
	}
	m, _ = m.Update(msgs[0])
	if m.submitting || !m.Active || !strings.Contains(m.View(), "resizing server") {
		t.Fatalf("error not surfaced/unlocked: submitting=%v\n%s", m.submitting, m.View())
	}
	rec.fail = false
	m, cmd = m.Update(keyEnter)
	if !m.submitting {
		t.Fatal("retry not accepted")
	}
	run(cmd)
	if rec.count() != 2 {
		t.Fatalf("retry posts=%d", rec.count())
	}
	m, _ = m.Update(resizeErrMsg{err: errors.New("x")})
	m, _ = m.Update(keyEsc)
	if m.Active {
		t.Fatal("escape after error should close")
	}
}

func TestTrackHookGuardsSubmission(t *testing.T) {
	m, rec := newBulk(t)
	var gotIDs []string
	m.Track = func(ids []string, cmd tea.Cmd) (tea.Cmd, string) {
		gotIDs = ids
		return nil, "srv-b already has an operation in progress"
	}
	m, _ = m.Update(keyDown)
	m, _ = m.Update(keyEnter)
	m, cmd := m.Update(keyY)
	if cmd != nil || m.submitting || !m.Active {
		t.Fatalf("refused resize still submitted: cmd=%v submitting=%v active=%v", cmd != nil, m.submitting, m.Active)
	}
	if strings.Join(gotIDs, ",") != "srv-a,srv-b" {
		t.Fatalf("Track ids=%v", gotIDs)
	}
	if !strings.Contains(m.View(), "in progress") {
		t.Fatalf("refusal not shown:\n%s", m.View())
	}

	// An accepting hook's command replaces the request.
	wrapped := false
	m.Track = func(ids []string, cmd tea.Cmd) (tea.Cmd, string) {
		return func() tea.Msg { wrapped = true; return cmd() }, ""
	}
	m, _ = m.Update(keyEnter)
	m, cmd = m.Update(keyY)
	if !m.submitting {
		t.Fatal("accepted resize not submitting")
	}
	run(cmd)
	if !wrapped || rec.count() != 2 {
		t.Fatalf("wrapped=%v posts=%d", wrapped, rec.count())
	}
}

// A bulk resize is audited once per server.
func TestBulkResizeAuditsEachServer(t *testing.T) {
	m, _ := newBulk(t)
	m, _ = m.Update(keyDown)
	m, _ = m.Update(keyEnter)
	_, cmd := m.Update(keyY)
	msgs := run(cmd)
	if len(msgs) != 1 {
		t.Fatalf("msgs=%v", msgs)
	}
	recs, ok := msgs[0].(shared.Auditable).TakeAudit()
	if !ok || len(recs) != 2 {
		t.Fatalf("records = %+v", recs)
	}
	for i, id := range []string{"srv-a", "srv-b"} {
		r := recs[i]
		if r.Action != audit.ActionResize || r.ResourceID != id || r.Err != nil || r.Details["flavor_id"] != "f-large" {
			t.Errorf("record %d = %+v", i, r)
		}
	}
}
