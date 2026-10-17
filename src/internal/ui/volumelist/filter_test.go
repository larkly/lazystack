package volumelist

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/larkly/lazystack/internal/volume"
)

func runeKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}) }

var (
	enterKey = tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	escKey   = tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
)

func filterVols() []volume.Volume {
	return []volume.Volume{
		{ID: "v1", Name: "web-data", Status: "in-use"},
		{ID: "v2", Name: "db-data", Status: "available"},
		{ID: "v3", Name: "web-logs", Status: "available"},
	}
}

func typeFilter(m Model, q string) Model {
	m, _ = m.Update(runeKey('/'))
	for _, r := range q {
		m, _ = m.Update(runeKey(r))
	}
	m, _ = m.Update(enterKey)
	return m
}

func TestFilterNarrowsListAndPersistsAcrossRefresh(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.SetSize(120, 30)
	m, _ = m.Update(volumesLoadedMsg{volumes: filterVols()})
	m = typeFilter(m, "WEB")
	if m.IsFiltering() || m.FilterValue() != "WEB" {
		t.Fatalf("filter not committed: filtering=%v value=%q", m.IsFiltering(), m.FilterValue())
	}
	if len(m.volumes) != 2 {
		t.Fatalf("listed %d volumes, want the 2 web ones", len(m.volumes))
	}
	// Put the cursor on web-logs, then refresh with a reordered list.
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	want := m.SelectedVolume().ID
	vols := filterVols()
	vols[0], vols[2] = vols[2], vols[0]
	m, _ = m.Update(volumesLoadedMsg{volumes: append(vols, volume.Volume{ID: "v4", Name: "web-new"})})
	if m.FilterValue() != "WEB" || len(m.volumes) != 3 {
		t.Fatalf("after refresh: filter=%q listed=%d, want WEB and 3", m.FilterValue(), len(m.volumes))
	}
	if got := m.SelectedVolume().ID; got != want {
		t.Fatalf("cursor on %s after refresh, want %s", got, want)
	}
	if !strings.Contains(ansi.Strip(m.View()), "filter: WEB") {
		t.Fatal("committed filter not shown")
	}
}

func TestFilterMatchesAttachedServerName(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.SetSize(120, 30)
	vols := filterVols()
	vols[1].AttachedServerID = "srv-1"
	m, _ = m.Update(volumesLoadedMsg{volumes: vols})
	m, _ = m.Update(serverNamesMsg{"srv-1": "postgres-01"})
	m = typeFilter(m, "postgres")
	if len(m.volumes) != 1 || m.volumes[0].ID != "v2" {
		t.Fatalf("listed %+v, want only v2", m.volumes)
	}
}

func TestEscClearsCommittedFilter(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.SetSize(120, 30)
	m, _ = m.Update(volumesLoadedMsg{volumes: filterVols()})
	m = typeFilter(m, "db")
	m, _ = m.Update(escKey)
	if m.FilterValue() != "" || len(m.volumes) != 3 {
		t.Fatalf("esc: filter=%q listed=%d, want cleared and 3", m.FilterValue(), len(m.volumes))
	}
}

func TestFilterWithNoMatchesSaysSo(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.SetSize(120, 30)
	m, _ = m.Update(volumesLoadedMsg{volumes: filterVols()})
	m = typeFilter(m, "zzz")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No volumes match") {
		t.Fatalf("view does not say nothing matches:\n%s", v)
	}
}

func TestFilterLineKeepsTableHeight(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.SetSize(120, 20)
	vols := make([]volume.Volume, 40)
	for i := range vols {
		vols[i] = volume.Volume{ID: string(rune('a' + i)), Name: "vol"}
	}
	m, _ = m.Update(volumesLoadedMsg{volumes: vols})
	m = typeFilter(m, "vol")
	if lines := strings.Count(m.View(), "\n"); lines > 20-2 {
		t.Fatalf("view is %d lines with the filter line, more than the %d available", lines, 20-2)
	}
}
