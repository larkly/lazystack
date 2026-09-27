package keypairlist

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
)

func pairs(n int) []compute.KeyPair {
	out := make([]compute.KeyPair, n)
	for i := range out {
		out[i] = compute.KeyPair{Name: fmt.Sprintf("key-%03d", i), Type: "ssh"}
	}
	return out
}

// checkViewport asserts the view fits the rows the app gives it (terminal
// height minus the tab bar and status bar) and shows the selected row.
func checkViewport(t *testing.T, m Model, label string) {
	t.Helper()
	view := strings.TrimSuffix(m.View(), "\n")
	lines := strings.Split(view, "\n")
	if avail := m.height - 2; len(lines) > avail {
		t.Fatalf("%s: %d lines rendered, %d available", label, len(lines), avail)
	}
	if kp := m.SelectedKeyPair(); kp != nil && !strings.Contains(ansi.Strip(view), "▸ "+kp.Name) {
		t.Fatalf("%s: selected %s not visible (cursor=%d scroll=%d)", label, kp.Name, m.cursor, m.scrollOff)
	}
}

func TestKeypairListViewportAndNavigation(t *testing.T) {
	m := New(nil, 0)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m, _ = m.Update(keypairsLoadedMsg{keypairs: pairs(50)})
	checkViewport(t, m, "initial")

	for i := 0; i < 60; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		checkViewport(t, m, fmt.Sprintf("down %d", i))
	}
	if m.SelectedKeyPair().Name != "key-049" {
		t.Fatalf("did not reach final keypair: %s", m.SelectedKeyPair().Name)
	}
	for _, k := range []tea.KeyPressMsg{{Code: tea.KeyPgUp}, {Code: tea.KeyPgUp}, {Code: tea.KeyUp}, {Code: tea.KeyPgUp}, {Code: tea.KeyPgUp}, {Code: tea.KeyPgUp}, {Code: tea.KeyPgDown}, {Code: tea.KeyPgDown}} {
		m, _ = m.Update(k)
		checkViewport(t, m, k.String())
	}
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	checkViewport(t, m, "pgdown to end")
	if m.SelectedKeyPair().Name != "key-049" {
		t.Fatalf("pgdown did not reach final keypair: %s", m.SelectedKeyPair().Name)
	}

	// Resize shrinks the viewport: cursor stays visible.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	checkViewport(t, m, "shrink")
	m.SetSize(80, 40)
	checkViewport(t, m, "grow")
	if m.scrollOff > len(m.pairs)-m.tableHeight() {
		t.Fatalf("scroll %d leaves blank rows after grow", m.scrollOff)
	}

	// Refresh with fewer pairs clamps cursor and scroll.
	m.SetSize(80, 12)
	m, _ = m.Update(keypairsLoadedMsg{keypairs: pairs(3)})
	if m.cursor != 2 || m.scrollOff != 0 {
		t.Fatalf("after refresh cursor=%d scroll=%d", m.cursor, m.scrollOff)
	}
	checkViewport(t, m, "refresh")
}

// Background ticks must not stack list fetches, and a slow older response
// must never overwrite the result of a newer fetch.
func TestTickRefreshesDoNotOverlapOrRegress(t *testing.T) {
	m := New(nil, time.Second)
	m.loading = false

	m, first := m.Update(shared.TickMsg{})
	if first == nil {
		t.Fatal("tick did not fetch")
	}
	staleSeq := m.refresh.Seq()
	if _, again := m.Update(shared.TickMsg{}); again != nil {
		t.Fatal("tick started a second fetch while one was in flight")
	}

	// A manual refresh supersedes the slow tick fetch.
	m.ForceRefresh()
	newSeq := m.refresh.Seq()
	m, _ = m.Update(keypairsLoadedMsg{seq: newSeq, keypairs: []compute.KeyPair{{Name: "new"}}})
	m, _ = m.Update(keypairsLoadedMsg{seq: staleSeq, keypairs: []compute.KeyPair{{Name: "old"}}})
	m, _ = m.Update(keypairsErrMsg{seq: staleSeq, err: errors.New("stale failure")})
	if len(m.pairs) != 1 || m.pairs[0].Name != "new" || m.err != "" {
		t.Fatalf("stale response applied: %v err=%q", m.pairs, m.err)
	}
	if _, next := m.Update(shared.TickMsg{}); next == nil {
		t.Fatal("tick blocked after the newest fetch completed")
	}
}
