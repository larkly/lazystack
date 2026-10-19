package subnetcreate

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/larkly/lazystack/internal/testutil"
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

func TestGatewayOpt(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want *string // nil: omit so Neutron assigns one
	}{
		{"", nil},
		{"   ", nil},
		{"none", ptr("")},
		{"NONE", ptr("")},
		{" None ", ptr("")},
		{"10.0.0.254", ptr("10.0.0.254")},
		{" 2001:db8::1 ", ptr("2001:db8::1")},
	} {
		got := gatewayOpt(tc.in)
		switch {
		case tc.want == nil && got != nil:
			t.Errorf("gatewayOpt(%q) = %q, want nil", tc.in, *got)
		case tc.want != nil && (got == nil || *got != *tc.want):
			t.Errorf("gatewayOpt(%q) = %v, want %q", tc.in, got, *tc.want)
		}
	}
}

func ptr(s string) *string { return &s }

// Typing "none" must create the subnet with gateway_ip null.
func TestSubmitNoneCreatesSubnetWithoutGateway(t *testing.T) {
	gateway := make(chan string, 1)
	client, closeFn := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Subnet map[string]json.RawMessage `json:"subnet"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, ok := body.Subnet["gateway_ip"]
		if !ok {
			raw = []byte("<absent>")
		}
		gateway <- string(raw)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"subnet":{"id":"sub","network_id":"net","cidr":"10.0.0.0/24","ip_version":4}}`))
	}))
	defer closeFn()

	m := New(client, "net", "net")
	m.cidrInput.SetValue("10.0.0.0/24")
	m.gatewayInput.SetValue("None")
	m, cmd := m.submit()
	if m.err != "" || cmd == nil {
		t.Fatalf("submit: err=%q cmd=%v", m.err, cmd)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("submit did not return a batch")
	}
	for _, c := range batch {
		if msg := c(); msg != nil {
			if _, ok := msg.(subnetCreatedMsg); ok {
				break
			}
		}
	}
	select {
	case got := <-gateway:
		if got != "null" {
			t.Errorf("gateway_ip = %s, want null", got)
		}
	default:
		t.Fatal("no create request was sent")
	}
}

func TestGatewayPlaceholderExplainsKeywords(t *testing.T) {
	m := New(nil, "net", "net")
	want := `IP, blank = auto, "none" = no gateway`
	if m.gatewayInput.Placeholder != want {
		t.Errorf("placeholder = %q, want %q", m.gatewayInput.Placeholder, want)
	}
	if !strings.Contains(ansi.Strip(m.View()), want) {
		t.Errorf("view does not show the full placeholder %q", want)
	}
}
