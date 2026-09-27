package volumelist

import (
	"strings"
	"testing"
	"time"

	"github.com/larkly/lazystack/internal/volume"
)

func loadedModel(vols ...volume.Volume) Model {
	m := New(nil, nil, time.Minute)
	m.SetSize(160, 40)
	m, _ = m.Update(volumesLoadedMsg{volumes: vols})
	return m
}

// --- #319: descending sort ---

func TestDescendingVolumeSortKeepsEqualRowsStable(t *testing.T) {
	m := loadedModel(
		volume.Volume{ID: "a", Name: "same"},
		volume.Volume{ID: "b", Name: "same"},
		volume.Volume{ID: "c", Name: "other"},
		volume.Volume{ID: "d", Name: "same"},
	)
	m.sortCol = 0 // name
	m.sortAsc = false
	m.cursor = 1 // "b"
	for round := 0; round < 3; round++ {
		m.sortVolumes()
		var ids []string
		for _, v := range m.volumes {
			ids = append(ids, v.ID)
		}
		if got := strings.Join(ids, ""); got != "abdc" {
			t.Fatalf("round %d: descending order = %q, want abdc", round, got)
		}
		if m.SelectedVolume().ID != "b" {
			t.Fatalf("round %d: cursor moved to %q", round, m.SelectedVolume().ID)
		}
	}
}

func TestVolumeSortNeverOrdersEqualValuesBothWays(t *testing.T) {
	x := volume.Volume{ID: "x", Name: "n", Status: "available", Size: 1, VolumeType: "ssd", AttachedDevice: "/dev/vdb", Bootable: "true"}
	y := x
	y.ID = "y"
	m := loadedModel(x, y)
	for col := 0; col < m.visibleColCount(); col++ {
		if m.visibleColKey(col) == "id" {
			continue // IDs differ by construction
		}
		for _, asc := range []bool{true, false} {
			m.sortCol, m.sortAsc = col, asc
			m.sortVolumes()
			if m.volumes[0].ID != "x" || m.volumes[1].ID != "y" {
				t.Fatalf("col %s asc=%v reordered equal rows", m.visibleColKey(col), asc)
			}
		}
	}
}

// --- #323: selection reconciliation ---

func TestVolumeRefreshPrunesStaleSelections(t *testing.T) {
	m := loadedModel(volume.Volume{ID: "v1", Name: "one"}, volume.Volume{ID: "v2", Name: "two"})
	m.selected["v1"] = true
	m.selected["v2"] = true

	m, _ = m.Update(volumesLoadedMsg{volumes: []volume.Volume{{ID: "v3", Name: "three"}}})
	if m.SelectionCount() != 0 || len(m.selected) != 0 {
		t.Fatalf("stale selections kept: count=%d map=%v", m.SelectionCount(), m.selected)
	}
	if got := m.SelectedVolumes(); len(got) != 1 || got[0].ID != "v3" {
		t.Fatalf("SelectedVolumes = %+v, want cursor volume", got)
	}
	if strings.Contains(m.Hints(), "selected)") {
		t.Fatalf("hints %q still advertise a selection", m.Hints())
	}
}

func TestVolumeSelectionCountMatchesTargets(t *testing.T) {
	m := loadedModel(volume.Volume{ID: "v1", Name: "one"}, volume.Volume{ID: "v2", Name: "two"})
	m.selected["v1"] = true
	m.selected["gone"] = true // e.g. deleted elsewhere before the next refresh

	if got := len(m.SelectedVolumes()); got != m.SelectionCount() || got != 1 {
		t.Fatalf("SelectedVolumes = %d, SelectionCount = %d; want 1 and equal", got, m.SelectionCount())
	}
}
