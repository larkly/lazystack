package serverrebuild

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

func press(m Model, k tea.Key) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg(k))
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = press(m, tea.Key{Code: r, Text: string(r)})
	}
	return m
}

func testImages() []image.Image {
	return []image.Image{
		{ID: "i-1", Name: "ubuntu-24.04"},
		{ID: "i-2", Name: "debian-12", MinDisk: 10, MinRAM: 512},
		{ID: "i-3", Name: "fedora-41"},
	}
}

func loaded() Model {
	m := New(nil, nil, "srv-1", "web", "i-1")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(imagesLoadedMsg{images: testImages()})
	return m
}

func TestNewIsLoadingAndActive(t *testing.T) {
	m := New(nil, nil, "srv-1", "web", "i-1")
	if !m.Active || !m.loading || m.cursor != 0 {
		t.Errorf("Active=%v loading=%v cursor=%d, want active, loading, cursor 0", m.Active, m.loading, m.cursor)
	}
}

func TestLoadedImagesRenderAndMarkCurrent(t *testing.T) {
	m := loaded()
	if m.loading || len(m.filtered) != 3 {
		t.Fatalf("loading=%v filtered=%d, want loaded with 3 images", m.loading, len(m.filtered))
	}
	v := m.View()
	for _, want := range []string{"ubuntu-24.04", "debian-12", "fedora-41", "★", "10GB", "512MB"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}
}

func TestUpDownMoveAndClampCursor(t *testing.T) {
	m := loaded()
	m, _ = press(m, tea.Key{Code: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("up at top: cursor=%d, want 0", m.cursor)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyDown})
	m, _ = press(m, tea.Key{Code: tea.KeyDown})
	m, _ = press(m, tea.Key{Code: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("down past end: cursor=%d, want clamped to 2", m.cursor)
	}
}

func TestEnterWithNoImagesDoesNothing(t *testing.T) {
	m := New(nil, nil, "srv-1", "web", "i-1")
	m, _ = m.Update(imagesLoadedMsg{})
	m, cmd := press(m, tea.Key{Code: tea.KeyEnter})
	if cmd != nil || m.confirming || m.submitting {
		t.Errorf("cmd=%v confirming=%v submitting=%v, want nothing", cmd != nil, m.confirming, m.submitting)
	}
	if !strings.Contains(m.View(), "No images found.") {
		t.Error("view should say no images were found")
	}
}

func TestEnterAsksForConfirmationBeforeRebuilding(t *testing.T) {
	m := loaded()
	m, _ = press(m, tea.Key{Code: tea.KeyDown})
	m, cmd := press(m, tea.Key{Code: tea.KeyEnter})
	if cmd != nil || !m.confirming || m.submitting {
		t.Fatalf("cmd=%v confirming=%v submitting=%v, want a pending confirmation only", cmd != nil, m.confirming, m.submitting)
	}
	if m.confirmImage != "debian-12" {
		t.Errorf("confirmImage=%q, want debian-12", m.confirmImage)
	}
	if v := m.View(); !strings.Contains(v, "Rebuild with debian-12?") || !strings.Contains(v, "destructive") {
		t.Error("view should show the destructive-action prompt")
	}
	if !strings.HasPrefix(m.Hints(), "y confirm") {
		t.Errorf("hints = %q, want confirmation hints", m.Hints())
	}
}

func TestConfirmYesStartsRebuild(t *testing.T) {
	m := loaded()
	m, _ = press(m, tea.Key{Code: tea.KeyEnter})
	m, cmd := press(m, tea.Key{Code: 'y', Text: "y"})
	if cmd == nil || !m.submitting {
		t.Errorf("cmd=%v submitting=%v, want rebuild started", cmd != nil, m.submitting)
	}
}

func TestConfirmNoOrEscBacksOut(t *testing.T) {
	for name, k := range map[string]tea.Key{
		"n":   {Code: 'n', Text: "n"},
		"esc": {Code: tea.KeyEscape},
	} {
		m := loaded()
		m, _ = press(m, tea.Key{Code: tea.KeyEnter})
		m, cmd := press(m, k)
		if cmd != nil || m.confirming || m.submitting || !m.Active {
			t.Errorf("%s: cmd=%v confirming=%v submitting=%v Active=%v, want back at the list", name, cmd != nil, m.confirming, m.submitting, m.Active)
		}
	}
}

func TestFilterNarrowsListAndEscClearsIt(t *testing.T) {
	m := loaded()
	m, _ = press(m, tea.Key{Code: '/', Text: "/"})
	if !m.filtering {
		t.Fatal("/ should open the filter")
	}
	m = typeText(m, "deb")
	if len(m.filtered) != 1 || m.filtered[0].Name != "debian-12" {
		t.Fatalf("filtered = %v, want only debian-12", m.filtered)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyEscape})
	if m.filtering || len(m.filtered) != 3 || !m.Active {
		t.Errorf("filtering=%v filtered=%d Active=%v, want filter cleared and modal still open", m.filtering, len(m.filtered), m.Active)
	}
}

func TestEscCancels(t *testing.T) {
	m := loaded()
	m, cmd := press(m, tea.Key{Code: tea.KeyEscape})
	if m.Active || cmd != nil {
		t.Errorf("Active=%v cmd=%v, want closed with no command", m.Active, cmd != nil)
	}
}

func TestKeysIgnoredWhileSubmitting(t *testing.T) {
	m := loaded()
	m, _ = press(m, tea.Key{Code: tea.KeyEnter})
	m, _ = press(m, tea.Key{Code: 'y', Text: "y"})
	m, cmd := press(m, tea.Key{Code: tea.KeyEscape})
	if !m.Active || cmd != nil {
		t.Error("esc must be ignored while submitting")
	}
}

func TestRebuildErrorShownAndConfirmationReset(t *testing.T) {
	m := loaded()
	m, _ = press(m, tea.Key{Code: tea.KeyEnter})
	m, _ = m.Update(rebuildErrMsg{err: errors.New("boom")})
	if m.confirming || m.submitting || m.err == "" || !m.Active {
		t.Errorf("confirming=%v submitting=%v err=%q Active=%v, want error with modal open", m.confirming, m.submitting, m.err, m.Active)
	}
	if !strings.Contains(m.View(), m.err) {
		t.Error("view should render the error")
	}
}

func TestFetchErrorKeepsModalOpenWithError(t *testing.T) {
	m := New(nil, nil, "srv-1", "web", "i-1")
	m, _ = m.Update(fetchErrMsg{err: errors.New("boom")})
	if m.loading || m.err == "" || !m.Active {
		t.Errorf("loading=%v err=%q Active=%v, want error shown", m.loading, m.err, m.Active)
	}
}

func TestRebuildDoneReportsServerAction(t *testing.T) {
	m := loaded()
	m, cmd := m.Update(rebuildDoneMsg{name: "web"})
	if m.Active || cmd == nil {
		t.Fatalf("Active=%v cmd=%v, want closed with result", m.Active, cmd != nil)
	}
	got, ok := cmd().(shared.ServerActionMsg)
	if !ok || got.Action != "Rebuilt" || got.Name != "web" {
		t.Errorf("result = %#v, want Rebuilt web", got)
	}
}

func TestViewAtNarrowWidth(t *testing.T) {
	m := loaded()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	testutil.AssertRenderSafe(t, m.View(), 40)
	m, _ = press(m, tea.Key{Code: tea.KeyEnter})
	testutil.AssertRenderSafe(t, m.View(), 40)
}
