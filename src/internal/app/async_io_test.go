package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/ui/copypicker"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
)

// updateWithin runs m.Update(msg) and fails if it blocks.
func updateWithin(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	type result struct {
		m   Model
		cmd tea.Cmd
	}
	ch := make(chan *result, 1)
	go func() {
		next, cmd := m.Update(msg)
		ch <- &result{next.(Model), cmd}
	}()
	select {
	case r := <-ch:
		return r.m, r.cmd
	case <-time.After(2 * time.Second):
		t.Fatalf("Update(%T) blocked on slow I/O", msg)
		return m, nil
	}
}

func TestCloudListingRunsOffTheUpdateLoop(t *testing.T) {
	release := make(chan struct{})
	old := listCloudNames
	listCloudNames = func() ([]string, error) {
		<-release
		return []string{"alpha", "beta"}, nil
	}
	t.Cleanup(func() { listCloudNames = old })

	m := initTestModel()
	m.view = viewServerList
	m, first := updateWithin(t, m, press("C"))
	if m.view != viewCloudPicker || first == nil {
		t.Fatal("C should open the cloud picker and list clouds asynchronously")
	}
	// A second open supersedes the first listing.
	m.view = viewServerList
	m, second := updateWithin(t, m, press("C"))
	close(release)
	staleResult, currentResult := first(), second()

	m, _ = updateWithin(t, m, staleResult)
	if strings.Contains(m.cloudPicker.View(), "alpha") {
		t.Fatal("a superseded cloud listing reset the picker")
	}
	m, _ = updateWithin(t, m, currentResult)
	if !strings.Contains(m.cloudPicker.View(), "alpha") {
		t.Fatal("cloud listing result was not applied to the picker")
	}
	// A listing that completes after the user left the picker is ignored.
	m.view = viewServerList
	m, cmd := updateWithin(t, m, press("C"))
	m.view = viewServerList
	m, _ = updateWithin(t, m, cmd())
	if m.view != viewServerList {
		t.Fatal("late cloud listing switched the view back to the picker")
	}
}

func TestClipboardWritesRunOffTheUpdateLoop(t *testing.T) {
	release := make(chan struct{})
	var written []string
	old := writeClipboard
	writeClipboard = func(s string) error {
		<-release
		written = append(written, s)
		return nil
	}
	t.Cleanup(func() { writeClipboard = old })

	m := initTestModel()
	m.view = viewServerList
	m, first := updateWithin(t, m, copypicker.ChosenMsg{Label: "ID", Value: "first"})
	if first == nil {
		t.Fatal("copy should complete through a command")
	}
	m, second := updateWithin(t, m, copypicker.ChosenMsg{Label: "ID", Value: "second"})
	close(release)
	older, newer := first(), second()
	m, _ = updateWithin(t, m, newer)
	if !strings.Contains(m.statusBar.StickyHint, "second") {
		t.Fatalf("copy result not reported: %q", m.statusBar.StickyHint)
	}
	m, _ = updateWithin(t, m, older)
	if !strings.Contains(m.statusBar.StickyHint, "second") {
		t.Fatalf("older copy result overwrote newer feedback: %q", m.statusBar.StickyHint)
	}
	if len(written) != 2 {
		t.Fatalf("clipboard writes = %v", written)
	}
}

func TestCopySSHCommandRunsOffTheUpdateLoop(t *testing.T) {
	release := make(chan struct{})
	old := writeClipboard
	writeClipboard = func(string) error { <-release; return errTest }
	t.Cleanup(func() { writeClipboard = old })

	m := initTestModel()
	m.view = viewServerDetail
	m.serverDetail = serverdetail.New(nil, nil, nil, "srv", time.Hour)
	m.serverDetail.SetServer(&compute.Server{ID: "srv", Name: "web", IPv6: []string{"2001:db8::1"}})
	m, cmd := updateWithin(t, m, press("y"))
	if cmd == nil {
		t.Fatal("y should copy through a command")
	}
	close(release)
	m, _ = updateWithin(t, m, cmd())
	if !strings.Contains(m.statusBar.StickyHint, "Clipboard error") {
		t.Fatalf("clipboard failure not reported: %q", m.statusBar.StickyHint)
	}
}
