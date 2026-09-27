package cloneprogress

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/testutil"
)

// recorder is a fake Nova/Cinder backend that records every request.
type recorder struct {
	mu       sync.Mutex
	requests []string
	handle   func(w http.ResponseWriter, r *http.Request) bool
}

func (rec *recorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec.mu.Lock()
	rec.requests = append(rec.requests, r.Method+" "+r.URL.Path)
	rec.mu.Unlock()
	if rec.handle != nil && rec.handle(w, r) {
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
}

func (rec *recorder) deletes() []string {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var out []string
	for _, r := range rec.requests {
		if strings.HasPrefix(r, "DELETE ") {
			out = append(out, r)
		}
	}
	return out
}

// run executes a command tree and returns the messages produced promptly.
// Timer commands slower than the wait (spinner frames, real poll intervals)
// are abandoned, so tests never sleep for real intervals.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(50 * time.Millisecond):
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, run(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func newPair(t *testing.T, rec *recorder) (a, b Model) {
	t.Helper()
	client, cleanup := testutil.FakeServiceClient(rec)
	t.Cleanup(cleanup)
	a = New(client, client, "A-server", "a", []VolumeOp{
		{SourceVolID: "src-1", CloneName: "a-1", Status: "pending"},
		{SourceVolID: "src-2", CloneName: "a-2", Status: "pending"},
		{SourceVolID: "src-3", CloneName: "a-3", Status: "pending"},
	})
	b = New(client, client, "B-server", "b", []VolumeOp{
		{SourceVolID: "src-9", CloneName: "b-1", Status: "creating", CloneVolID: "B-vol"},
	})
	return a, b
}

func TestLateMessagesFromAnotherCloneAreIgnored(t *testing.T) {
	rec := &recorder{}
	a, b := newPair(t, rec)
	// A's volume creation fails for its third volume (index 2), which is out
	// of range for B and must never trigger B's rollback.
	for _, idx := range []int{0, 2} {
		msg := a.createVolume(idx, a.volumes[idx])()
		next, cmd := b.Update(msg)
		if cmd != nil {
			for _, m := range run(cmd) {
				_ = m
			}
		}
		if next.failed || next.rollingBack || next.volumes[0].Status != "creating" {
			t.Fatalf("A's result (idx %d) mutated B: %+v", idx, next.volumes[0])
		}
		b = next
	}
	if d := rec.deletes(); len(d) != 0 {
		t.Fatalf("stale clone messages deleted resources: %v", d)
	}
}

var errBoom = errors.New("boom")

func TestOutOfRangeIndicesAreRejected(t *testing.T) {
	rec := &recorder{}
	_, b := newPair(t, rec)
	for _, idx := range []int{-1, 1, 1 << 30} {
		for _, msg := range []tea.Msg{
			volumeCreatedMsg{op: b.op, idx: idx, err: errBoom},
			volumeCreatedMsg{op: b.op, idx: idx, volID: "x"},
			volumeStatusMsg{op: b.op, idx: idx, status: "available"},
			volumeStatusMsg{op: b.op, idx: idx, err: errBoom},
			volumeAttachedMsg{op: b.op, idx: idx, err: errBoom},
			volumeAttachedMsg{op: b.op, idx: idx},
		} {
			next, cmd := b.Update(msg)
			if cmd != nil || next.failed || next.volumes[0].Status != "creating" {
				t.Fatalf("idx %d accepted: %T", idx, msg)
			}
		}
	}
}

func TestRollbackRunsOnceAndOnlyForOwnResources(t *testing.T) {
	rec := &recorder{handle: func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusAccepted)
			return true
		}
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return true
		}
		return false
	}}
	_, b := newPair(t, rec)
	b, cmd := b.Update(volumeStatusMsg{op: b.op, idx: 0, status: "error"})
	if cmd == nil || !b.rollingBack {
		t.Fatal("volume error did not start rollback")
	}
	if again, second := b.Update(volumeAttachedMsg{op: b.op, idx: 0, err: errBoom}); second != nil || !again.rollingBack {
		t.Fatal("rollback started twice")
	}
	var completed bool
	for _, msg := range run(cmd) {
		var done tea.Cmd
		b, done = b.Update(msg)
		for _, out := range run(done) {
			rc, ok := out.(RollbackCompleteMsg)
			if !ok || rc.Op != b.ID() {
				t.Fatalf("completion %T does not identify clone %d", out, b.ID())
			}
			completed = true
		}
	}
	if !completed || b.Running() {
		t.Fatal("rollback did not complete")
	}
	deletes := rec.deletes()
	if len(deletes) == 0 {
		t.Fatal("rollback deleted nothing")
	}
	for _, d := range deletes {
		if !strings.HasSuffix(d, "/B-vol") && !strings.HasSuffix(d, "/B-server") {
			t.Fatalf("rollback touched resources it does not own: %s", d)
		}
	}
	// A late failure after rollback finished must not start another one.
	if _, cmd := b.Update(volumeStatusMsg{op: b.op, idx: 0, err: errBoom}); cmd != nil {
		t.Fatal("finished clone reacted to a late message")
	}
}

// statusBackend serves fixed server and volume statuses.
func statusBackend(serverStatus string, volumeStatus map[string]string) *recorder {
	return &recorder{handle: func(w http.ResponseWriter, r *http.Request) bool {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			return false
		}
		if strings.HasPrefix(r.URL.Path, "/servers/") {
			fmt.Fprintf(w, `{"server":{"id":"B-server","name":"b","status":%q,"flavor":{"id":"f"}}}`, serverStatus)
			return true
		}
		id := strings.TrimPrefix(r.URL.Path, "/volumes/")
		if st, ok := volumeStatus[id]; ok {
			fmt.Fprintf(w, `{"volume":{"id":%q,"status":%q}}`, id, st)
			return true
		}
		return false
	}}
}

func withFastPoll(t *testing.T) {
	t.Helper()
	old := pollInterval
	pollInterval = time.Millisecond
	t.Cleanup(func() { pollInterval = old })
}

// pollRounds delivers poll ticks round by round and returns how many ticks
// each round scheduled for the next one.
func pollRounds(t *testing.T, m Model, rounds int) (Model, []int) {
	t.Helper()
	pending := []tea.Msg{pollTickMsg{op: m.op}}
	var counts []int
	for r := 0; r < rounds && len(pending) > 0; r++ {
		queue := pending
		pending = nil
		for len(queue) > 0 {
			msg := queue[0]
			queue = queue[1:]
			var cmd tea.Cmd
			m, cmd = m.Update(msg)
			for _, out := range run(cmd) {
				if _, ok := out.(pollTickMsg); ok {
					pending = append(pending, out)
				} else {
					queue = append(queue, out)
				}
			}
		}
		counts = append(counts, len(pending))
	}
	return m, counts
}

func TestPollKeepsASingleChainWhileServerBoots(t *testing.T) {
	withFastPoll(t)
	rec := statusBackend("BUILD", map[string]string{"v0": "available", "v1": "creating"})
	client, cleanup := testutil.FakeServiceClient(rec)
	t.Cleanup(cleanup)
	m := New(client, client, "B-server", "b", []VolumeOp{
		{CloneName: "b-0", CloneVolID: "v0", Status: "available"},
		{CloneName: "b-1", CloneVolID: "v1", Status: "creating"},
	})
	m.pendingAttach = []int{0}
	m.polling = true
	m, counts := pollRounds(t, m, 5)
	for i, c := range counts {
		if c != 1 {
			t.Fatalf("round %d scheduled %d poll ticks, want exactly 1 (all rounds: %v)", i, c, counts)
		}
	}
	if !m.Running() || m.failed {
		t.Fatal("clone should still be waiting for the server")
	}
}

func TestPollChainStopsOnceClonesAreAttached(t *testing.T) {
	withFastPoll(t)
	rec := statusBackend("ACTIVE", map[string]string{"v0": "available"})
	inner := rec.handle
	rec.handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/os-volume_attachments") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"volumeAttachment":{"id":"att","volumeId":"v0","serverId":"B-server"}}`)
			return true
		}
		return inner(w, r)
	}
	client, cleanup := testutil.FakeServiceClient(rec)
	t.Cleanup(cleanup)
	m := New(client, client, "B-server", "b", []VolumeOp{{CloneName: "b-0", Status: "pending"}})
	m, cmd := m.Update(volumeCreatedMsg{op: m.op, idx: 0, volID: "v0"})
	if cmd == nil || !m.polling {
		t.Fatal("created volume should start polling")
	}
	var completed bool
	queue := []tea.Msg{pollTickMsg{op: m.op}}
	for len(queue) > 0 {
		inFlight := 0
		for _, q := range queue {
			if _, ok := q.(pollTickMsg); ok {
				inFlight++
			}
		}
		if inFlight > 1 {
			t.Fatalf("%d poll ticks in flight at once", inFlight)
		}
		msg := queue[0]
		queue = queue[1:]
		if _, ok := msg.(AllCompleteMsg); ok {
			completed = true
			continue
		}
		var out tea.Cmd
		m, out = m.Update(msg)
		queue = append(queue, run(out)...)
	}
	if !completed || m.Running() {
		t.Fatal("clone did not complete")
	}
	if m.polling {
		t.Fatal("poll chain kept running after completion")
	}
}
