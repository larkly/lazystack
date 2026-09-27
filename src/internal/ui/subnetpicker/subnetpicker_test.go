package subnetpicker

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"strings"
	"testing"
)

func TestSelectValidateAndCancel(t *testing.T) {
	m := New(nil, "router", "edge")
	m.SetSize(80, 25)
	if m.Init() == nil || !strings.Contains(m.View(), "Loading") {
		t.Fatal("loading state")
	}
	m, _ = m.Update(subnetsLoadedMsg{[]network.Subnet{{ID: "subnet-one", Name: "private", CIDR: "10.0.0.0/24", GatewayIP: "10.0.0.254"}, {ID: "subnet-two", Name: "other", CIDR: "192.0.2.0/24"}}})
	for _, code := range []rune{tea.KeyUp, tea.KeyDown, tea.KeyDown} {
		m, _ = m.Update(tea.KeyPressMsg{Code: code})
	}
	if m.cursor != 1 {
		t.Fatal("navigation boundary")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.phase != phaseConfirm || m.ipInput.Value() != "192.0.2.1" || !m.ipInput.Focused() {
		t.Fatal("default IP or phase")
	}
	for _, tc := range []struct{ ip, want string }{{"", "required"}, {"garbage", "Invalid"}, {"10.0.0.1", "not within"}} {
		m.ipInput.SetValue(tc.ip)
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		if cmd != nil || m.submitting || !strings.Contains(m.View(), tc.want) {
			t.Fatalf("ip=%q err=%q", tc.ip, m.err)
		}
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.phase != phaseList || !m.Active || m.err != "" {
		t.Fatal("cancel confirmation")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.ipInput.Value() != "10.0.0.254" {
		t.Fatal("gateway default")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.Active {
		t.Fatal("cancel list")
	}
}
func TestSubmissionAndResult(t *testing.T) {
	for _, mode := range []string{"", "slaac"} {
		m := New(nil, "router", "edge")
		m, _ = m.Update(subnetsLoadedMsg{[]network.Subnet{{ID: "subnet-one", Name: "private", CIDR: "10.0.0.0/24", IPv6AddressMode: mode}}})
		m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if mode == "" {
			m, cmd = m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		}
		if !m.submitting || cmd == nil || !strings.Contains(m.View(), "Adding interface") {
			t.Fatal("not submitting")
		}
		m, _ = m.Update(interfaceAddErrMsg{err: errors.New("denied")})
		if m.submitting || !strings.Contains(m.View(), "denied") {
			t.Fatal("error state")
		}
		m, cmd = m.Update(interfaceAddedMsg{routerName: "edge"})
		if m.Active || cmd == nil {
			t.Fatal("completion")
		}
		if got := cmd().(shared.ResourceActionMsg); got.Action != "Added interface to" || got.Name != "edge" {
			t.Fatalf("message=%+v", got)
		}
	}
}
func TestEmptyAndFetchError(t *testing.T) {
	m := New(nil, "router", "edge")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 45, Height: 20})
	m, _ = m.Update(subnetsLoadedMsg{})
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.phase != phaseList || !strings.Contains(m.View(), "No subnets") {
		t.Fatal("empty state")
	}
	m, _ = m.Update(fetchErrMsg{errors.New("denied")})
	if m.loading || !strings.Contains(m.View(), "denied") {
		t.Fatal("fetch error")
	}
}
