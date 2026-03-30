package lbmembercreate

import (
	"testing"

	"github.com/larkly/lazystack/internal/compute"
)

func TestAdvanceFocusSkipsManualAddressWhenServerSourceSelected(t *testing.T) {
	m := New(nil, nil, "pool-1", "pool-1", "2001:db8::10", nil)
	m.addressSource = addressSourceServer
	m.focusField = fieldAddrSource

	m.advanceFocus(1)

	if m.focusField != fieldServer {
		t.Fatalf("focusField = %d, want fieldServer (%d)", m.focusField, fieldServer)
	}
}

func TestNewStartsOnSourceField(t *testing.T) {
	m := New(nil, nil, "pool-1", "pool-1", "10.0.0.10", nil)

	if m.focusField != fieldAddrSource {
		t.Fatalf("focusField = %d, want fieldAddrSource (%d)", m.focusField, fieldAddrSource)
	}
}

func TestAdvanceFocusMovesFromServerToName(t *testing.T) {
	m := New(nil, nil, "pool-1", "pool-1", "10.0.0.10", nil)
	m.addressSource = addressSourceServer
	m.focusField = fieldServer

	m.advanceFocus(1)

	if m.focusField != fieldName {
		t.Fatalf("focusField = %d, want fieldName (%d)", m.focusField, fieldName)
	}
}

func TestSubmitAcceptsSelectedServerAddress(t *testing.T) {
	m := New(nil, nil, "pool-1", "pool-1", "10.0.0.10", nil)
	m.addressSource = addressSourceServer
	m.serverOptions = []memberServerOption{{id: "srv-1", name: "srv-1", address: "10.0.0.5"}}
	m.selectedServerID = "srv-1"
	m.portInput.SetValue("8080")
	m.weightInput.SetValue("2")

	updated, cmd := m.submit()

	if updated.err != "" {
		t.Fatalf("submit error = %q, want empty", updated.err)
	}
	if !updated.submitting {
		t.Fatal("expected submitting to be true")
	}
	if cmd == nil {
		t.Fatal("expected submit command")
	}
}

func TestSubmitRejectsInvalidManualIP(t *testing.T) {
	m := New(nil, nil, "pool-1", "pool-1", "10.0.0.10", nil)
	m.addressSource = addressSourceIP
	m.addrInput.SetValue("not-an-ip")
	m.portInput.SetValue("8080")

	updated, cmd := m.submit()

	if updated.err == "" {
		t.Fatal("expected validation error for invalid IP")
	}
	if cmd != nil {
		t.Fatal("expected no command on validation failure")
	}
}

func TestPreferredMemberAddressUsesRequestedFamily(t *testing.T) {
	srv := compute.Server{
		IPv4:       []string{"10.0.0.5"},
		IPv6:       []string{"2001:db8::5"},
		FloatingIP: []string{"203.0.113.5"},
	}

	addr := preferredMemberAddress(srv, 6)

	if addr != "2001:db8::5" {
		t.Fatalf("preferredMemberAddress = %q, want 2001:db8::5", addr)
	}
}

func TestApplyServerFilterMatchesNameIDAndAddress(t *testing.T) {
	m := New(nil, nil, "pool-1", "pool-1", "10.0.0.10", nil)
	m.serverOptions = []memberServerOption{
		{id: "srv-1", name: "alpha", address: "10.0.0.5"},
		{id: "srv-2", name: "beta", address: "10.0.0.6"},
	}
	m.selectedServerID = "srv-2"
	m.serverFilter = "10.0.0.6"

	m.applyServerFilter()

	if len(m.filteredServers) != 1 {
		t.Fatalf("filteredServers len = %d, want 1", len(m.filteredServers))
	}
	if m.filteredServers[0].id != "srv-2" {
		t.Fatalf("filtered server id = %q, want srv-2", m.filteredServers[0].id)
	}
	if m.pickerCursor != 0 {
		t.Fatalf("pickerCursor = %d, want 0", m.pickerCursor)
	}
}

func TestSelectServerDefaultsNameWhenEmpty(t *testing.T) {
	m := New(nil, nil, "pool-1", "pool-1", "10.0.0.10", nil)
	srv := memberServerOption{id: "srv-1", name: "web-01", address: "10.0.0.5"}

	m.selectServer(srv)

	if m.selectedServerID != "srv-1" {
		t.Fatalf("selectedServerID = %q, want srv-1", m.selectedServerID)
	}
	if m.nameInput.Value() != "web-01" {
		t.Fatalf("nameInput = %q, want web-01", m.nameInput.Value())
	}
}

func TestSelectServerKeepsExistingName(t *testing.T) {
	m := New(nil, nil, "pool-1", "pool-1", "10.0.0.10", nil)
	m.nameInput.SetValue("custom-name")

	m.selectServer(memberServerOption{id: "srv-1", name: "web-01", address: "10.0.0.5"})

	if m.nameInput.Value() != "custom-name" {
		t.Fatalf("nameInput = %q, want custom-name", m.nameInput.Value())
	}
}

func TestMakeAddressSetSkipsEmptyValues(t *testing.T) {
	set := makeAddressSet([]string{"10.0.0.5", "", "  ", "2001:db8::5"})

	if len(set) != 2 {
		t.Fatalf("set len = %d, want 2", len(set))
	}
	if _, ok := set["10.0.0.5"]; !ok {
		t.Fatal("expected 10.0.0.5 in set")
	}
	if _, ok := set["2001:db8::5"]; !ok {
		t.Fatal("expected 2001:db8::5 in set")
	}
}
