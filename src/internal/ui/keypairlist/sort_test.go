package keypairlist

import (
	"testing"
	"time"

	"github.com/larkly/lazystack/internal/compute"
)

func names(pairs []compute.KeyPair) []string {
	out := make([]string, len(pairs))
	for i, kp := range pairs {
		out[i] = kp.Name
	}
	return out
}

func TestDescendingSortKeepsTiesStable(t *testing.T) {
	m := New(nil, time.Second)
	m.pairs = []compute.KeyPair{
		{Name: "a", Type: "ssh"},
		{Name: "b", Type: "x509"},
		{Name: "c", Type: "ssh"},
		{Name: "d", Type: "ssh"},
	}
	m.sortCol = 1 // type
	m.sortAsc = false
	m.sortPairs()

	want := []string{"b", "a", "c", "d"}
	got := names(m.pairs)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("descending by type = %v, want %v (ties must keep input order)", got, want)
		}
	}
}

func TestAscendingSortByName(t *testing.T) {
	m := New(nil, time.Second)
	m.pairs = []compute.KeyPair{{Name: "Charlie"}, {Name: "alpha"}, {Name: "bravo"}}
	m.sortCol = 0
	m.sortAsc = true
	m.sortPairs()
	want := []string{"alpha", "bravo", "Charlie"}
	got := names(m.pairs)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ascending by name = %v, want %v", got, want)
		}
	}
}
