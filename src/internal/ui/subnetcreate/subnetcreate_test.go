package subnetcreate

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestIPv6ModeValue(t *testing.T) {
	tests := []struct {
		idx  int
		want string
	}{
		{idx: 0, want: "slaac"},
		{idx: 1, want: "dhcpv6-stateful"},
		{idx: 2, want: "dhcpv6-stateless"},
		{idx: 3, want: ""},
		{idx: 99, want: ""},
	}

	for _, tc := range tests {
		if got := ipv6ModeValue(tc.idx); got != tc.want {
			t.Fatalf("ipv6ModeValue(%d)=%q want %q", tc.idx, got, tc.want)
		}
	}
}

func hiddenForIPv4(f int) bool { return f >= fieldPrefixLen && f <= fieldIPv6RA }

func TestIPv4NavigationNeverFocusesHiddenFields(t *testing.T) {
	keys := map[string]tea.KeyPressMsg{
		"enter":     {Code: tea.KeyEnter},
		"tab":       {Code: tea.KeyTab},
		"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
		"up":        {Code: tea.KeyUp},
		"down":      {Code: tea.KeyDown},
	}
	for start := 0; start < numFields; start++ {
		if hiddenForIPv4(start) || start == fieldSubmit || start == fieldCancel {
			continue
		}
		for name, k := range keys {
			m := New(nil, "net", "net")
			m.focusField = start
			m.updateFocus()
			m, _ = m.Update(k)
			if hiddenForIPv4(m.focusField) {
				t.Errorf("IPv4: %s from field %d focused hidden field %d", name, start, m.focusField)
			}
		}
	}
}

func TestIPv6NavigationVisitsAllFieldsInOrder(t *testing.T) {
	m := New(nil, "net", "net")
	m.focusField = fieldIPVersion
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if m.ipVersion != 1 || m.focusField != fieldIPVersion {
		t.Fatalf("toggle: version=%d focus=%d", m.ipVersion, m.focusField)
	}
	m.focusField = fieldName
	m.updateFocus()
	var order []int
	for range numFields {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		order = append(order, m.focusField)
	}
	want := []int{fieldIPVersion, fieldSubnetPool, fieldCIDR, fieldGateway, fieldDHCP, fieldPrefixLen, fieldIPv6Cfg, fieldIPv6RA, fieldSubmit, fieldCancel, fieldName}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("tab order = %v, want %v", order, want)
		}
	}
	// Enter walks the same visible order.
	m.focusField = fieldDHCP
	for _, w := range []int{fieldPrefixLen, fieldIPv6Cfg, fieldIPv6RA, fieldSubmit} {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.focusField != w {
			t.Fatalf("enter reached %d, want %d", m.focusField, w)
		}
	}
	// Switching back to IPv4 from the version field keeps focus visible and
	// the next Tab skips the IPv6-only fields.
	m.focusField = fieldIPVersion
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m.focusField = fieldDHCP
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.ipVersion != 0 || m.focusField != fieldSubmit {
		t.Fatalf("after switching to IPv4: version=%d focus=%d", m.ipVersion, m.focusField)
	}
}
