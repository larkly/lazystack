package fippicker

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
)

const twoExtNets = `{"networks":[{"id":"ext-0","name":"backup"},{"id":"ext-1","name":"public"}]}`

// With no free floating IP the picker offers the allocate row and does
// nothing until Enter.
func TestNoFreeFIPDoesNotAutoAllocate(t *testing.T) {
	f := newFakeFIPNeutron()
	f.fips = `{"floatingips":[]}`
	client, cleanup := testutil.FakeServiceClient(f.handler(t))
	defer cleanup()

	m := New(client, "srv", "web")
	m.SetSize(80, 30)
	m, _ = drive(t, m, m.Init())
	if f.creates != 0 || m.submitting {
		t.Fatalf("allocated on load: creates=%d submitting=%v", f.creates, m.submitting)
	}
	if !strings.Contains(m.View(), "+ Allocate new floating IP") {
		t.Fatalf("allocate row not offered:\n%s", m.View())
	}

	m, cmd := m.Update(enter)
	m, emitted := drive(t, m, cmd)
	if f.creates != 1 || m.Active || len(emitted) != 1 {
		t.Fatalf("enter did not allocate: creates=%d active=%v emitted=%v", f.creates, m.Active, emitted)
	}
}

// Several external networks require choosing one; the routed network is
// preselected and Esc returns to the floating IP list.
func TestAllocateAsksForNetworkWhenSeveralExist(t *testing.T) {
	f := newFakeFIPNeutron()
	f.fips = `{"floatingips":[]}`
	f.extNets = twoExtNets
	client, cleanup := testutil.FakeServiceClient(f.handler(t))
	defer cleanup()

	m := New(client, "srv", "web")
	m.SetSize(80, 30)
	m, _ = drive(t, m, m.Init())
	m, cmd := m.Update(enter)
	m, _ = drive(t, m, cmd)
	if f.creates != 0 || !m.choosingNet {
		t.Fatalf("allocated without a network choice: creates=%d", f.creates)
	}
	view := m.View()
	for _, want := range []string{"backup", "public"} {
		if !strings.Contains(view, want) {
			t.Fatalf("network step missing %q:\n%s", want, view)
		}
	}
	if m.netChoices[m.netCursor].ID != "ext-1" {
		t.Fatalf("preselected %q, want the routed ext-1", m.netChoices[m.netCursor].ID)
	}

	back, _ := m.Update(esc)
	if !back.Active || back.choosingNet || !strings.Contains(back.View(), "+ Allocate new floating IP") {
		t.Fatalf("esc should return to the list:\n%s", back.View())
	}

	m, cmd = m.Update(enter)
	m, emitted := drive(t, m, cmd)
	if f.creates != 1 || !strings.Contains(f.lastAlloc, `"floating_network_id":"ext-1"`) {
		t.Fatalf("allocation = %d %s, want one from ext-1", f.creates, f.lastAlloc)
	}
	if m.Active || len(emitted) != 1 {
		t.Fatalf("expected completion, active=%v emitted=%v", m.Active, emitted)
	}
}

// The allocate-only picker used by the floating IP list allocates from the
// only external network directly and asks when there are several.
func TestAllocateOnly(t *testing.T) {
	f := newFakeFIPNeutron()
	client, cleanup := testutil.FakeServiceClient(f.handler(t))
	defer cleanup()

	m := NewAllocate(client)
	m.SetSize(80, 30)
	m, emitted := drive(t, m, m.Init())
	if f.creates != 1 || m.Active || len(emitted) != 1 {
		t.Fatalf("single network: creates=%d active=%v emitted=%v", f.creates, m.Active, emitted)
	}
	if got := emitted[0].(shared.ResourceActionMsg); got.Action != "Allocated" || got.Name != "198.51.100.7" {
		t.Fatalf("result = %+v", got)
	}

	f.extNets = twoExtNets
	m = NewAllocate(client)
	m.SetSize(80, 30)
	m, _ = drive(t, m, m.Init())
	if f.creates != 1 || !m.choosingNet || !strings.Contains(m.View(), "backup") {
		t.Fatalf("several networks: creates=%d view:\n%s", f.creates, m.View())
	}
	m, _ = m.Update(down)
	m, cmd := m.Update(enter)
	m, _ = drive(t, m, cmd)
	if f.creates != 2 || !strings.Contains(f.lastAlloc, `"floating_network_id":"ext-1"`) || m.Active {
		t.Fatalf("allocation = %d %s active=%v", f.creates, f.lastAlloc, m.Active)
	}

	m = NewAllocate(client)
	m, _ = drive(t, m, m.Init())
	m, _ = m.Update(esc)
	if m.Active {
		t.Fatal("esc in the network step should close the allocate-only picker")
	}
}

// Choosing a network no router connects to the server falls through to the
// address step with a warning; Esc there returns to the list.
func TestAllocateOnUnroutedNetworkAsksForAddress(t *testing.T) {
	f := newFakeFIPNeutron()
	f.fips = `{"floatingips":[]}`
	f.extNets = twoExtNets
	client, cleanup := testutil.FakeServiceClient(f.handler(t))
	defer cleanup()

	m := New(client, "srv", "web")
	m.SetSize(100, 30)
	m, _ = drive(t, m, m.Init())
	m, cmd := m.Update(enter)
	m, _ = drive(t, m, cmd)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m, cmd = m.Update(enter)
	m, _ = drive(t, m, cmd)
	if f.creates != 0 || !m.choosingPort || m.pendingExtID != "ext-0" || m.portWarning == "" {
		t.Fatalf("creates=%d choosingPort=%v ext=%q warning=%q", f.creates, m.choosingPort, m.pendingExtID, m.portWarning)
	}
	m, _ = m.Update(esc)
	if !m.Active || m.choosingPort || !strings.Contains(m.View(), "+ Allocate new floating IP") {
		t.Fatalf("esc should return to the list:\n%s", m.View())
	}
}

// Both modes share the app's picker slot, so a load result from a picker
// closed while loading can reach a newly opened one of the other mode. It
// must be ignored: a stale network list must not allocate for a server, and
// a stale floating IP list must not appear in the allocate-only picker.
func TestLoadResultsFromTheOtherModeAreIgnored(t *testing.T) {
	nets := []network.Network{{ID: "ext-1", Name: "public"}}

	srv := New(nil, "srv", "web")
	srv, cmd := srv.Update(netsLoadedMsg{nets: nets})
	if cmd != nil || srv.submitting || srv.choosingNet || !srv.loading {
		t.Fatalf("server picker acted on a network list: cmd=%v submitting=%v", cmd != nil, srv.submitting)
	}
	srv, _ = srv.Update(fetchErrMsg{allocOnly: true, err: errors.New("stale")})
	if srv.err != "" || !srv.loading {
		t.Fatalf("server picker took the allocate picker's error: %q", srv.err)
	}

	alloc := NewAllocate(nil)
	alloc, cmd = alloc.Update(fipsLoadedMsg{fips: []network.FloatingIP{{ID: "fip", FloatingIP: "192.0.2.1"}}})
	if cmd != nil || len(alloc.fips) != 0 || !alloc.loading {
		t.Fatalf("allocate picker took a floating IP list: fips=%v", alloc.fips)
	}
	alloc, _ = alloc.Update(fetchErrMsg{err: errors.New("stale")})
	if alloc.err != "" || !alloc.loading {
		t.Fatalf("allocate picker took the server picker's error: %q", alloc.err)
	}
}

// Allocation and loading errors are shown in their friendly form.
func TestAllocateErrorsAreFriendly(t *testing.T) {
	raw := errors.New("Expected HTTP response code [201] when accessing [POST http://neutron:9696/v2.0/floatingips], but got 409 instead: Quota exceeded for resources: ['floatingip']")
	want := shared.ParseError(raw).FriendlyMessage
	for _, msg := range []tea.Msg{fetchErrMsg{allocOnly: true, err: raw}, allocateErrMsg{err: raw}} {
		m := NewAllocate(nil)
		m, _ = m.Update(msg)
		if m.err != want {
			t.Fatalf("%T: err = %q, want %q", msg, m.err, want)
		}
	}
}

// The network step degrades its glyphs in plain mode.
func TestNetworkStepPlainGlyphs(t *testing.T) {
	orig := shared.PlainMode
	shared.PlainMode = true
	t.Cleanup(func() { shared.PlainMode = orig })

	m := NewAllocate(nil)
	m.SetSize(80, 30)
	m, _ = m.Update(netsLoadedMsg{nets: []network.Network{{ID: "a", Name: "one"}, {ID: "b", Name: "two"}}})
	view := ansi.Strip(m.netChoiceView())
	if strings.ContainsAny(view, "▸↑↓") || !strings.Contains(view, "> one") || !strings.Contains(view, "up/down") {
		t.Fatalf("plain view:\n%s", view)
	}
}
