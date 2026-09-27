package imageview

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	img "github.com/larkly/lazystack/internal/image"
)

// --- #319: descending sort ---

func TestDescendingImageSortKeepsEqualRowsStable(t *testing.T) {
	m := loadedModel(
		img.Image{ID: "a", Name: "same"},
		img.Image{ID: "b", Name: "same"},
		img.Image{ID: "c", Name: "other"},
		img.Image{ID: "d", Name: "same"},
	)
	m.sortCol = 0 // name
	m.sortAsc = false
	for round := 0; round < 3; round++ {
		m.sortImages()
		var ids []string
		for _, im := range m.images {
			ids = append(ids, im.ID)
		}
		if got := strings.Join(ids, ""); got != "abdc" {
			t.Fatalf("round %d: descending order = %q, want abdc", round, got)
		}
	}
}

func TestImageSortNeverOrdersEqualValuesBothWays(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	x := img.Image{ID: "x", Name: "n", Status: "active", Size: 1, MinDisk: 1, MinRAM: 1, Visibility: "public", CreatedAt: ts}
	y := x
	y.ID = "y"
	m := loadedModel(x, y)
	for col := 0; col < m.visibleColCount(); col++ {
		for _, asc := range []bool{true, false} {
			m.sortCol, m.sortAsc = col, asc
			m.sortImages()
			if m.images[0].ID != "x" || m.images[1].ID != "y" {
				t.Fatalf("col %s asc=%v reordered equal rows", m.visibleColKey(col), asc)
			}
		}
	}
}

// --- #323: selection reconciliation ---

func TestImageSelectionCountMatchesActionTargetsWhenFiltered(t *testing.T) {
	m := loadedModel(namedImages("alpha", "beta", "gamma")...)
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	}
	if m.SelectionCount() != 3 {
		t.Fatalf("SelectionCount = %d, want 3", m.SelectionCount())
	}

	m.searchFilter = "et" // hides alpha and gamma
	if got := len(m.SelectedImages()); got != m.SelectionCount() {
		t.Fatalf("SelectedImages = %d, SelectionCount = %d; want equal", got, m.SelectionCount())
	}
	if m.SelectionCount() != 1 || m.SelectedImages()[0].ID != "id-beta" {
		t.Fatalf("targets = %+v, want only visible beta", m.SelectedImages())
	}

	// All selections hidden: no bulk count, cursor action targets the visible image.
	m.ClearSelection()
	m.searchFilter = ""
	m.selected["id-alpha"] = true
	m.searchFilter = "gamma"
	m.cursor = 0
	if m.SelectionCount() != 0 {
		t.Fatalf("SelectionCount = %d with only hidden selections, want 0", m.SelectionCount())
	}
	if got := m.SelectedImages(); len(got) != 1 || got[0].ID != "id-gamma" {
		t.Fatalf("SelectedImages = %+v, want cursor image gamma", got)
	}
	if strings.Contains(m.Hints(), "selected") {
		t.Fatalf("hints %q advertise a hidden selection", m.Hints())
	}
}

func TestImageRefreshPrunesStaleSelections(t *testing.T) {
	m := loadedModel(namedImages("alpha", "beta")...)
	m.selected["id-alpha"] = true
	m.selected["id-beta"] = true

	m, _ = m.Update(imagesLoadedMsg{images: namedImages("gamma")})
	if m.SelectionCount() != 0 || len(m.selected) != 0 {
		t.Fatalf("stale selections kept: count=%d map=%v", m.SelectionCount(), m.selected)
	}
	if got := m.SelectedImages(); len(got) != 1 || got[0].ID != "id-gamma" {
		t.Fatalf("SelectedImages = %+v, want cursor image", got)
	}
}
