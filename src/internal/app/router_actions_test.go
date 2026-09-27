package app

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/modal"
	"github.com/larkly/lazystack/internal/ui/routerview"
)

type routerFixtureState struct {
	executing bool
	mutations []string
	// portIPs is served for GET /ports/<id> during execution.
	portIPs map[string]string
	// portDevice overrides the device_id served for GET /ports/<id>.
	portDevice map[string]string
	deleteCode int
}

// routerFixture loads a router with one interface per subnet in subnets
// (port "p-<subnet>") into the router view.
func routerFixture(t *testing.T, st *routerFixtureState, subnets ...string) Model {
	t.Helper()
	var ports []string
	for i, sub := range subnets {
		ports = append(ports, fmt.Sprintf(`{"id":"p-%s","device_id":"router","device_owner":"network:router_interface","fixed_ips":[{"subnet_id":%q,"ip_address":"10.0.%d.1"}]}`, sub, sub, i))
	}
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if st.executing && r.Method != http.MethodGet {
			body, _ := io.ReadAll(r.Body)
			st.mutations = append(st.mutations, r.Method+" "+r.URL.Path+" "+strings.TrimSpace(string(body)))
			switch {
			case r.Method == http.MethodDelete && st.deleteCode != 0:
				w.WriteHeader(st.deleteCode)
				fmt.Fprint(w, `{"NeutronError":{"type":"RouterInUse","message":"Router router still has ports"}}`)
			case r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			case strings.HasPrefix(r.URL.Path, "/ports/"):
				fmt.Fprint(w, `{"port":{"id":"x"}}`)
			default:
				fmt.Fprint(w, `{"id":"router"}`)
			}
			return
		}
		switch {
		case r.URL.Path == "/routers":
			fmt.Fprint(w, `{"routers":[{"id":"router","name":"fixture-router"}]}`)
		case r.URL.Path == "/ports":
			fmt.Fprintf(w, `{"ports":[%s]}`, strings.Join(ports, ","))
		case strings.HasPrefix(r.URL.Path, "/ports/"):
			id := strings.TrimPrefix(r.URL.Path, "/ports/")
			ips, ok := st.portIPs[id]
			if !ok {
				http.Error(w, "port not found", http.StatusNotFound)
				return
			}
			device := "router"
			if d, ok := st.portDevice[id]; ok {
				device = d
			}
			fmt.Fprintf(w, `{"port":{"id":%q,"device_id":%q,"device_owner":"network:router_interface","fixed_ips":%s}}`, id, device, ips)
		case r.URL.Path == "/networks":
			fmt.Fprint(w, `{"networks":[]}`)
		case r.URL.Path == "/subnets":
			fmt.Fprint(w, `{"subnets":[]}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	m.routerView = routerview.New(m.client.Network, m.refreshInterval)
	loadViewData(m.routerView.Init(), func(msg tea.Msg) tea.Cmd {
		var cmd tea.Cmd
		m.routerView, cmd = m.routerView.Update(msg)
		return cmd
	})
	m.view = viewRouterView
	return m
}

func focusRouterInterfaces(t *testing.T, m Model) Model {
	t.Helper()
	for range 4 {
		if m.routerView.SelectedInterface() != nil {
			return m
		}
		m.routerView, _ = m.routerView.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	}
	t.Fatal("could not focus router interfaces")
	return m
}

// confirmCurrent turns the open confirmation into the message the modal
// would send when the user confirms it.
func confirmCurrent(m Model) modal.ConfirmAction {
	return modal.ConfirmAction{Action: m.confirm.Action, ServerID: m.confirm.ServerID, Name: m.confirm.Name, Servers: m.confirm.Servers, Confirm: true}
}

func TestRouterDeleteConfirmationIsAccurate(t *testing.T) {
	st := &routerFixtureState{}
	m := routerFixture(t, st, "sub1", "sub2")
	m, _ = m.openRouterDeleteConfirm()
	if m.activeModal != modalConfirm {
		t.Fatal("no confirmation")
	}
	if strings.Contains(m.confirm.Body, "will be removed") {
		t.Fatalf("confirmation promises interface cleanup that is never performed: %q", m.confirm.Body)
	}
	if !strings.Contains(m.confirm.Body, "2 attached interface") {
		t.Fatalf("confirmation does not warn about attached interfaces: %q", m.confirm.Body)
	}

	st.executing, st.deleteCode = true, http.StatusConflict
	_, cmd := m.executeAction(confirmCurrent(m))
	msg := cmd()
	e, ok := msg.(shared.ResourceActionErrMsg)
	if !ok || !strings.Contains(shared.ParseError(e.Err).FriendlyMessage, "remove its interfaces first") {
		t.Fatalf("conflict result is not actionable: %#v", msg)
	}
	if len(st.mutations) != 1 || !strings.HasPrefix(st.mutations[0], "DELETE /routers/router") {
		t.Fatalf("router delete must not implicitly detach interfaces: %v", st.mutations)
	}
}

func TestRemoveRouterInterfaceUsesTargetCapturedAtConfirmation(t *testing.T) {
	st := &routerFixtureState{portIPs: map[string]string{
		"p-sub1": `[{"subnet_id":"sub1","ip_address":"10.0.0.1"}]`,
		"p-sub2": `[{"subnet_id":"sub2","ip_address":"10.0.1.1"}]`,
	}}
	m := routerFixture(t, st, "sub1", "sub2")
	auditPath := withAuditLog(t, &m)
	m = focusRouterInterfaces(t, m)
	if iface := m.routerView.SelectedInterface(); iface == nil || iface.SubnetID != "sub1" {
		t.Fatalf("fixture selected %+v", iface)
	}
	m, _ = m.openRemoveRouterInterfaceConfirm()
	if !strings.Contains(m.confirm.Body, "sub1") || !strings.Contains(m.confirm.Body, "10.0.0.1") {
		t.Fatalf("confirmation does not identify the interface: %q", m.confirm.Body)
	}
	// Background refresh / cursor movement changes the selection to sub2
	// while the dialog is open.
	m.routerView, _ = m.routerView.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if iface := m.routerView.SelectedInterface(); iface == nil || iface.SubnetID != "sub2" {
		t.Fatalf("selection did not move: %+v", iface)
	}
	st.executing = true
	_, cmd := m.executeAction(confirmCurrent(m))
	if cmd == nil {
		t.Fatal("no removal command")
	}
	if _, ok := cmd().(shared.ResourceActionMsg); !ok {
		t.Fatal("removal failed")
	}
	if len(st.mutations) != 1 || st.mutations[0] != `PUT /routers/router/remove_router_interface {"subnet_id":"sub1"}` {
		t.Fatalf("mutations=%v, want only the confirmed sub1 interface removed", st.mutations)
	}
	checkInterfaceAudit(t, auditPath, "success")
}

// withAuditLog points the model at a fresh, enabled audit log.
func withAuditLog(t *testing.T, m *Model) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "audit.log")
	m.auditLogger = audit.NewLogger(p, true)
	return p
}

func checkInterfaceAudit(t *testing.T, path, result string) {
	t.Helper()
	entries, err := audit.ReadEntries(path, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("audit=%+v err=%v", entries, err)
	}
	e := entries[0]
	if e.Action != audit.ActionRemoveInterface || e.ResourceType != "router" || e.ResourceID != "router" || e.Result != result ||
		e.Cloud != "test-cloud" || e.Project != "test-project" || !strings.Contains(string(e.Details), "sub1") {
		t.Errorf("audit=%+v details=%s", e, e.Details)
	}
	if (result == "error") != (e.Error != "") {
		t.Errorf("audit error=%q", e.Error)
	}
}

func TestRemoveRouterInterfaceRevalidatesPortAtExecution(t *testing.T) {
	for _, tc := range []struct {
		name, ips, device string
		wantMutation      string
	}{
		{"subnet gone", `[{"subnet_id":"other","ip_address":"10.9.0.1"}]`, "router", ""},
		{"port moved", `[{"subnet_id":"sub1","ip_address":"10.0.0.1"}]`, "another-router", ""},
		{"became multi-ip", `[{"subnet_id":"sub1","ip_address":"10.0.0.1"},{"subnet_id":"sub9","ip_address":"10.0.9.1"}]`, "router",
			`PUT /ports/p-sub1 {"port":{"fixed_ips":[{"ip_address":"10.0.9.1","subnet_id":"sub9"}]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &routerFixtureState{
				portIPs:    map[string]string{"p-sub1": `[{"subnet_id":"sub1","ip_address":"10.0.0.1"}]`},
				portDevice: map[string]string{},
			}
			m := routerFixture(t, st, "sub1")
			auditPath := withAuditLog(t, &m)
			m = focusRouterInterfaces(t, m)
			m, _ = m.openRemoveRouterInterfaceConfirm()
			st.portIPs["p-sub1"], st.portDevice["p-sub1"] = tc.ips, tc.device
			st.executing = true
			_, cmd := m.executeAction(confirmCurrent(m))
			msg := cmd()
			if tc.wantMutation == "" {
				if _, ok := msg.(shared.ResourceActionErrMsg); !ok || len(st.mutations) != 0 {
					t.Fatalf("stale target must fail without mutating: msg=%#v mutations=%v", msg, st.mutations)
				}
				checkInterfaceAudit(t, auditPath, "error")
				return
			}
			if _, ok := msg.(shared.ResourceActionMsg); !ok || len(st.mutations) != 1 || st.mutations[0] != tc.wantMutation {
				t.Fatalf("msg=%#v mutations=%v", msg, st.mutations)
			}
			checkInterfaceAudit(t, auditPath, "success")
		})
	}
}
