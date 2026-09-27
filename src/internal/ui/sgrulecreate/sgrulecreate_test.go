package sgrulecreate

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

// fakeRules is a Neutron security-group-rules endpoint holding one
// original rule. It records created rule bodies and deleted IDs.
type fakeRules struct {
	mu          sync.Mutex
	original    map[string]any
	getStatus   int
	postStatus  int
	deleteCodes map[string]int // rule ID -> status for DELETE
	posts       []map[string]any
	deletes     []string
}

func (f *fakeRules) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		switch r.Method {
		case http.MethodGet:
			if f.getStatus != 0 {
				http.Error(w, `{"NeutronError":{"message":"denied"}}`, f.getStatus)
				return
			}
			if id != f.original["id"] {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"security_group_rule": f.original})
		case http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]map[string]any
			_ = json.Unmarshal(raw, &body)
			f.posts = append(f.posts, body["security_group_rule"])
			if f.postStatus != 0 {
				http.Error(w, `{"NeutronError":{"message":"rejected"}}`, f.postStatus)
				return
			}
			rule := body["security_group_rule"]
			rule["id"] = "new-rule"
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"security_group_rule": rule})
		case http.MethodDelete:
			f.deletes = append(f.deletes, id)
			if code := f.deleteCodes[id]; code != 0 {
				http.Error(w, `{"NeutronError":{"message":"cannot delete"}}`, code)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
	}
}

// listRule converts a wire rule to the simplified model the security group
// view hands to NewEdit.
func listRule(raw map[string]any) network.SecurityRule {
	str := func(k string) string { s, _ := raw[k].(string); return s }
	num := func(k string) int { n, _ := raw[k].(float64); v, _ := raw[k].(int); return int(n) + v }
	return network.SecurityRule{
		ID: str("id"), Direction: str("direction"), EtherType: str("ethertype"), Protocol: str("protocol"),
		PortRangeMin: num("port_range_min"), PortRangeMax: num("port_range_max"),
		RemoteIPPrefix: str("remote_ip_prefix"), RemoteGroupID: str("remote_group_id"),
	}
}

func baseRule(extra map[string]any) map[string]any {
	r := map[string]any{
		"id": "old-rule", "security_group_id": "sg-1", "direction": "ingress", "ethertype": "IPv4",
		"protocol": "tcp", "port_range_min": 22, "port_range_max": 22,
		"remote_ip_prefix": nil, "remote_group_id": nil, "remote_address_group_id": nil,
	}
	for k, v := range extra {
		r[k] = v
	}
	return r
}

// drive runs cmd and feeds messages back into the model, collecting the
// messages meant for the root model.
func drive(m Model, cmd tea.Cmd) (Model, []tea.Msg) {
	var out []tea.Msg
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil, spinner.TickMsg:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case shared.ResourceActionMsg, shared.ResourceActionErrMsg:
			out = append(out, msg)
		default:
			var next tea.Cmd
			m, next = m.Update(msg)
			queue = append(queue, next)
		}
	}
	return m, out
}

func openEdit(t *testing.T, f *fakeRules) Model {
	t.Helper()
	client, cleanup := testutil.FakeServiceClient(f.handler(t))
	t.Cleanup(cleanup)
	m := NewEdit(client, "sg-1", "web", listRule(f.original))
	m.SetSize(100, 40)
	m, _ = drive(m, m.Init())
	return m
}

func submit(m Model) (Model, []tea.Msg) {
	m, cmd := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	return drive(m, cmd)
}

func setPortMax(m Model, v string) Model {
	m.portMaxInput.SetValue(v)
	return m
}

func TestReviewRemoteGroupRoundtrip(t *testing.T) {
	for _, ether := range []string{"IPv4", "IPv6"} {
		t.Run(ether, func(t *testing.T) {
			f := &fakeRules{original: baseRule(map[string]any{"ethertype": ether, "remote_group_id": "sg-peers"})}
			m := openEdit(t, f)
			_, out := submit(m)
			if len(f.posts) != 0 || len(f.deletes) != 0 {
				t.Fatalf("no-op edit wrote: posts=%v deletes=%v", f.posts, f.deletes)
			}
			if len(out) != 1 {
				t.Fatalf("no-op edit result = %v", out)
			}

			m = openEdit(t, f)
			m = setPortMax(m, "23")
			_, out = submit(m)
			if len(f.posts) != 1 {
				t.Fatalf("posts = %v", f.posts)
			}
			post := f.posts[0]
			if post["remote_group_id"] != "sg-peers" || post["remote_ip_prefix"] != nil || post["ethertype"] != ether {
				t.Fatalf("remote selector lost: %v", post)
			}
			if len(f.deletes) != 1 || f.deletes[0] != "old-rule" {
				t.Fatalf("deletes = %v", f.deletes)
			}
			if a, ok := out[0].(shared.ResourceActionMsg); !ok || a.Action != "Updated rule in" {
				t.Fatalf("result = %v", out)
			}
		})
	}
}

func TestReviewAddressGroupRoundtrip(t *testing.T) {
	f := &fakeRules{original: baseRule(map[string]any{"remote_address_group_id": "ag-office"})}
	m := openEdit(t, f)
	submit(m)
	if len(f.posts) != 0 || len(f.deletes) != 0 {
		t.Fatalf("no-op edit wrote: posts=%v deletes=%v", f.posts, f.deletes)
	}
	m = openEdit(t, f)
	m = setPortMax(m, "25")
	submit(m)
	if len(f.posts) != 1 || f.posts[0]["remote_address_group_id"] != "ag-office" || f.posts[0]["remote_ip_prefix"] != nil {
		t.Fatalf("address group lost: %v", f.posts)
	}
}

func TestRemoteSelectorsAreMutuallyExclusive(t *testing.T) {
	f := &fakeRules{original: baseRule(map[string]any{"remote_group_id": "sg-peers"})}
	m := openEdit(t, f)
	m.remoteIPInput.SetValue("0.0.0.0/0")
	m, out := submit(m)
	if len(f.posts) != 0 || len(f.deletes) != 0 || len(out) != 0 || m.err == "" || !m.Active {
		t.Fatalf("combined selectors submitted: posts=%v err=%q", f.posts, m.err)
	}
}

func TestRuleLoadFailureBlocksEdit(t *testing.T) {
	f := &fakeRules{original: baseRule(nil), getStatus: http.StatusForbidden}
	m := openEdit(t, f)
	m = setPortMax(m, "23")
	m, out := submit(m)
	if len(f.posts) != 0 || len(f.deletes) != 0 || len(out) != 0 || !strings.Contains(m.View(), "denied") {
		t.Fatalf("edit proceeded after load failure: posts=%v out=%v", f.posts, out)
	}
}

func TestCreateFailureKeepsOriginalRule(t *testing.T) {
	f := &fakeRules{original: baseRule(nil), postStatus: http.StatusConflict}
	m := openEdit(t, f)
	m = setPortMax(m, "23")
	m, out := submit(m)
	if len(f.deletes) != 0 || len(out) != 0 || m.err == "" || !m.Active {
		t.Fatalf("original touched after failed create: deletes=%v out=%v err=%q", f.deletes, out, m.err)
	}
}

func TestProtocolRoundtrip(t *testing.T) {
	cases := []struct {
		name  string
		extra map[string]any
	}{
		{"ipv6-icmp", map[string]any{"ethertype": "IPv6", "protocol": "ipv6-icmp", "port_range_min": 128, "port_range_max": nil}},
		{"icmpv6", map[string]any{"ethertype": "IPv6", "protocol": "icmpv6", "port_range_min": nil, "port_range_max": nil}},
		{"sctp", map[string]any{"protocol": "sctp", "port_range_min": 3868, "port_range_max": 3868}},
		{"numeric", map[string]any{"protocol": "132", "port_range_min": nil, "port_range_max": nil}},
		{"any", map[string]any{"protocol": nil, "port_range_min": nil, "port_range_max": nil}},
		{"icmp type 8 any code", map[string]any{"protocol": "icmp", "port_range_min": 8, "port_range_max": nil}},
		{"icmp type 0 code 0", map[string]any{"protocol": "icmp", "port_range_min": 0, "port_range_max": 0}},
		{"icmp any", map[string]any{"protocol": "icmp", "port_range_min": nil, "port_range_max": nil}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRules{original: baseRule(c.extra)}
			m := openEdit(t, f)
			if p, _ := f.original["protocol"].(string); p != "" && !strings.Contains(m.View(), p) {
				t.Fatalf("protocol %q not visible:\n%s", p, m.View())
			}
			// Change only the direction and check everything else survives.
			m.focusField = fieldDirection
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
			_, out := submit(m)
			if len(f.posts) != 1 {
				t.Fatalf("posts=%v out=%v err=%q", f.posts, out, m.err)
			}
			post := f.posts[0]
			if post["direction"] != "egress" {
				t.Fatalf("direction = %v", post["direction"])
			}
			for _, k := range []string{"protocol", "port_range_min", "port_range_max", "ethertype"} {
				want := f.original[k]
				got, present := post[k]
				if want == nil {
					if present && got != nil {
						t.Errorf("%s = %v, want unset", k, got)
					}
					continue
				}
				if !present || jsonEq(got) != jsonEq(want) {
					t.Errorf("%s = %v (present=%v), want %v", k, got, present, want)
				}
			}
		})
	}
}

func jsonEq(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestReplacementDeleteFailureIsReported(t *testing.T) {
	t.Run("rolled back", func(t *testing.T) {
		f := &fakeRules{original: baseRule(nil), deleteCodes: map[string]int{"old-rule": http.StatusForbidden}}
		m := openEdit(t, f)
		m = setPortMax(m, "23")
		m, out := submit(m)
		if len(out) != 0 || !m.Active {
			t.Fatalf("reported success: %v", out)
		}
		if strings.Join(f.deletes, ",") != "old-rule,new-rule" {
			t.Fatalf("deletes = %v, want old then rollback of new", f.deletes)
		}
		if !strings.Contains(m.err, "old-rule") || !strings.Contains(m.err, "new-rule") {
			t.Fatalf("error does not name both rules: %q", m.err)
		}
	})
	t.Run("rollback failed", func(t *testing.T) {
		f := &fakeRules{original: baseRule(nil), deleteCodes: map[string]int{"old-rule": http.StatusInternalServerError, "new-rule": http.StatusConflict}}
		m := openEdit(t, f)
		m = setPortMax(m, "23")
		_, out := submit(m)
		if len(out) != 1 {
			t.Fatalf("out = %v", out)
		}
		e, ok := out[0].(shared.ResourceActionErrMsg)
		if !ok || !strings.Contains(e.Err.Error(), "old-rule") || !strings.Contains(e.Err.Error(), "new-rule") {
			t.Fatalf("partial failure not explicit: %#v", out[0])
		}
	})
	t.Run("original already gone", func(t *testing.T) {
		f := &fakeRules{original: baseRule(nil), deleteCodes: map[string]int{"old-rule": http.StatusNotFound}}
		m := openEdit(t, f)
		m = setPortMax(m, "23")
		_, out := submit(m)
		if a, ok := out[0].(shared.ResourceActionMsg); !ok || a.Action != "Updated rule in" {
			t.Fatalf("out = %v", out)
		}
	})
}

func TestCreateRuleSendsExplicitBounds(t *testing.T) {
	f := &fakeRules{original: baseRule(nil)}
	client, cleanup := testutil.FakeServiceClient(f.handler(t))
	defer cleanup()

	m := New(client, "sg-1", "web")
	if m.Init() != nil {
		t.Fatal("create mode should not load anything")
	}
	m.portMinInput.SetValue("8080")
	_, out := submit(m)
	if len(f.posts) != 1 || len(f.deletes) != 0 || jsonEq(f.posts[0]["port_range_min"]) != "8080" || jsonEq(f.posts[0]["port_range_max"]) != "8080" {
		t.Fatalf("posts=%v deletes=%v", f.posts, f.deletes)
	}
	if a, ok := out[0].(shared.ResourceActionMsg); !ok || a.Action != "Created rule in" {
		t.Fatalf("out = %v", out)
	}

	// ICMP type 0 (echo reply) must be sent, not dropped as a wildcard.
	f.posts = nil
	m = New(client, "sg-1", "web")
	m.selectedProtocol = 2 // icmp
	m.portMinInput.SetValue("0")
	submit(m)
	if v, ok := f.posts[0]["port_range_min"]; !ok || jsonEq(v) != "0" {
		t.Fatalf("icmp type 0 lost: %v", f.posts[0])
	}
	if _, ok := f.posts[0]["port_range_max"]; ok {
		t.Fatalf("icmp code should be unset: %v", f.posts[0])
	}

	// Ports with a portless protocol and mismatched remote families are
	// rejected before any request.
	f.posts = nil
	m = New(client, "sg-1", "web")
	m.selectedProtocol = 3 // any
	m.portMinInput.SetValue("22")
	m, _ = submit(m)
	n := New(client, "sg-1", "web")
	n.remoteIPInput.SetValue("2001:db8::/64")
	n, _ = submit(n)
	if len(f.posts) != 0 || m.err == "" || n.err == "" {
		t.Fatalf("invalid input submitted: posts=%v errs=%q %q", f.posts, m.err, n.err)
	}
}
