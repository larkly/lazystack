package keypairdetail

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/compute"
)

func press(m Model, code rune, n int) Model {
	for i := 0; i < n; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: code})
	}
	return m
}

func TestScrollIsClampedInUpdate(t *testing.T) {
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, fmt.Sprintf("line%02d", i))
	}
	m := New(nil, "k")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m, _ = m.Update(keypairLoadedMsg{kp: &compute.KeyPairFull{Name: "k", Type: "ssh", PublicKey: strings.Join(lines, "\n")}})

	m = press(m, tea.KeyDown, 100)
	bottom := m.View()
	if !strings.Contains(bottom, "line19") {
		t.Fatalf("bottom view does not show the last line:\n%s", bottom)
	}
	atBottom := m.scroll

	m = press(m, tea.KeyUp, 1)
	if m.scroll != atBottom-1 {
		t.Fatalf("one Up after overscroll: scroll=%d, want %d", m.scroll, atBottom-1)
	}
	if m.View() == bottom {
		t.Fatal("one Up after overscroll did not move the view")
	}
}

func TestSingleLineKeyNeverScrolls(t *testing.T) {
	m := New(nil, "k")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m, _ = m.Update(keypairLoadedMsg{kp: &compute.KeyPairFull{Name: "k", PublicKey: "ssh-ed25519 AAAA k"}})
	m = press(m, tea.KeyDown, 5)
	if m.scroll != 0 {
		t.Fatalf("scroll=%d, want 0", m.scroll)
	}
}

func TestScrollBeforeLoadStaysZero(t *testing.T) {
	m := New(nil, "k")
	m = press(m, tea.KeyDown, 3)
	if m.scroll != 0 {
		t.Fatalf("scroll=%d before load, want 0", m.scroll)
	}
}

// PgDn/PgUp page through a long public key a screen at a time, like the
// volume detail view, and stay within the content.
func TestPageKeysScrollByAScreen(t *testing.T) {
	var lines []string
	for i := 0; i < 60; i++ {
		lines = append(lines, fmt.Sprintf("line%02d", i))
	}
	m := New(nil, "k")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m, _ = m.Update(keypairLoadedMsg{kp: &compute.KeyPairFull{Name: "k", Type: "ssh", PublicKey: strings.Join(lines, "\n")}})
	page := m.keyViewHeight()

	m = press(m, tea.KeyPgDown, 1)
	if m.scroll != page {
		t.Fatalf("PgDn: scroll=%d, want %d", m.scroll, page)
	}
	m = press(m, tea.KeyPgDown, 10)
	if m.scroll != m.maxScroll() {
		t.Fatalf("PgDn past the end: scroll=%d, want %d", m.scroll, m.maxScroll())
	}
	m = press(m, tea.KeyPgUp, 1)
	if m.scroll != m.maxScroll()-page {
		t.Fatalf("PgUp: scroll=%d, want %d", m.scroll, m.maxScroll()-page)
	}
	m = press(m, tea.KeyPgUp, 10)
	if m.scroll != 0 {
		t.Fatalf("PgUp past the top: scroll=%d, want 0", m.scroll)
	}
}
