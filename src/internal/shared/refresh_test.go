package shared

import "testing"

func TestRefreshGate(t *testing.T) {
	var g RefreshGate
	if g.Busy() {
		t.Fatal("zero gate is busy")
	}
	first := g.Start()
	if !g.Busy() {
		t.Fatal("not busy after Start")
	}
	second := g.Start() // e.g. a manual refresh while a tick fetch runs
	if g.Accept(first) {
		t.Fatal("stale response accepted")
	}
	if !g.Busy() {
		t.Fatal("stale response ended the newer fetch's in-flight period")
	}
	if !g.Accept(second) {
		t.Fatal("newest response rejected")
	}
	if g.Busy() {
		t.Fatal("still busy after the newest response")
	}
	if g.Accept(first) {
		t.Fatal("late stale response accepted after the newest one")
	}
}
