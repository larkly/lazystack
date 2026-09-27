package app

import (
	"fmt"
	"io"
	"net/http"
	"reflect"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/lbview"
	"github.com/larkly/lazystack/internal/ui/modal"
	"github.com/larkly/lazystack/internal/ui/routerview"
)

// Follow data-loaded commands, but never recursively schedule spinner ticks.
func loadViewData(cmd tea.Cmd, update func(tea.Msg) tea.Cmd) {
	for _, msg := range commandMessages(cmd) {
		if _, ok := msg.(spinner.TickMsg); ok {
			continue
		}
		loadViewData(update(msg), update)
	}
}

func TestBulkMemberDeletionWaitsAndAggregates(t *testing.T) {
	for _, mode := range []string{"success", "delete-error", "wait-error"} {
		t.Run(mode, func(t *testing.T) {
			executing := false
			var actions []string
			m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if executing {
					actions = append(actions, r.Method+" "+r.URL.Path)
				}
				if r.Method == "DELETE" {
					if mode == "delete-error" && r.URL.Path == "/lbaas/pools/pool/members/a" {
						http.Error(w, "member failed", http.StatusConflict)
					} else {
						w.WriteHeader(204)
					}
					return
				}
				switch r.URL.Path {
				case "/lbaas/loadbalancers":
					fmt.Fprint(w, `{"loadbalancers":[{"id":"lb","name":"fixture-lb","provisioning_status":"ACTIVE"}]}`)
				case "/lbaas/loadbalancers/lb":
					if executing && mode == "wait-error" {
						http.Error(w, "gone", 404)
						return
					}
					fmt.Fprint(w, `{"loadbalancer":{"id":"lb","provisioning_status":"ACTIVE"}}`)
				case "/lbaas/listeners":
					fmt.Fprint(w, `{"listeners":[]}`)
				case "/lbaas/pools":
					fmt.Fprint(w, `{"pools":[{"id":"pool","name":"fixture-pool"}]}`)
				case "/lbaas/pools/pool/members":
					fmt.Fprint(w, `{"members":[{"id":"a","name":"alpha"},{"id":"b","name":"beta"},{"id":"c","name":"gamma"}]}`)
				default:
					t.Errorf("unexpected %s", r.URL)
					w.WriteHeader(404)
				}
			})
			m.lbView = lbview.New(m.client.LoadBalancer, m.refreshInterval)
			loadViewData(m.lbView.Init(), func(msg tea.Msg) tea.Cmd { var cmd tea.Cmd; m.lbView, cmd = m.lbView.Update(msg); return cmd })
			m.lbView.ToggleAllMemberSelection()
			if m.lbView.SelectedMemberCount() != 3 {
				t.Fatalf("fixture selected %d members", m.lbView.SelectedMemberCount())
			}
			executing = true
			next, cmd := m.executeAction(modal.ConfirmAction{Action: "delete_lb_members_bulk", ServerID: "pool"})
			if next.lbView.SelectedMemberCount() != 0 {
				t.Fatal("bulk deletion retained selection")
			}
			msg := cmd()
			want := []string{"DELETE /lbaas/pools/pool/members/a", "GET /lbaas/loadbalancers/lb"}
			if mode != "wait-error" {
				want = append(want, "DELETE /lbaas/pools/pool/members/b", "GET /lbaas/loadbalancers/lb", "DELETE /lbaas/pools/pool/members/c")
			}
			if !reflect.DeepEqual(actions, want) {
				t.Fatalf("action order=%v want=%v", actions, want)
			}
			if mode == "success" {
				s, ok := msg.(shared.ResourceActionMsg)
				if !ok || s.Action != "Deleted 3 members from" || s.Name != "pool" {
					t.Fatalf("result=%#v", msg)
				}
			} else {
				e, ok := msg.(shared.ResourceActionErrMsg)
				failed := 1
				if mode == "wait-error" {
					failed = 2
				}
				if !ok || e.Name != fmt.Sprintf("%d of 3 members", failed) || e.Err.Error() != fmt.Sprintf("%d members failed to delete", failed) {
					t.Fatalf("result=%#v", msg)
				}
			}
		})
	}
}

func TestRouterInterfaceRemovalChoosesDetachOrFixedIPUpdate(t *testing.T) {
	for _, multi := range []bool{false, true} {
		for _, failed := range []bool{false, true} {
			t.Run(fmt.Sprintf("multi=%v/error=%v", multi, failed), func(t *testing.T) {
				executing := false
				mutations := 0
				ips := `[{"subnet_id":"sub1","ip_address":"10.0.0.1"}]`
				if multi {
					ips = `[{"subnet_id":"sub1","ip_address":"10.0.0.1"},{"subnet_id":"sub2","ip_address":"10.0.1.1"}]`
				}
				m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if executing && r.Method == "PUT" {
						mutations++
						body, _ := io.ReadAll(r.Body)
						if multi {
							if r.URL.Path != "/ports/port" {
								t.Errorf("wrong port endpoint %s", r.URL)
							}
							checkJSON(t, body, `{"port":{"fixed_ips":[{"subnet_id":"sub2","ip_address":"10.0.1.1"}]}}`)
						} else {
							if r.URL.Path != "/routers/router/remove_router_interface" {
								t.Errorf("wrong detach endpoint %s", r.URL)
							}
							checkJSON(t, body, `{"subnet_id":"sub1"}`)
						}
						if failed {
							http.Error(w, "removal failed", http.StatusConflict)
							return
						}
						if multi {
							fmt.Fprint(w, `{"port":{"id":"port"}}`)
						} else {
							fmt.Fprint(w, `{"id":"router","port_id":"port","subnet_id":"sub1"}`)
						}
						return
					}
					switch r.URL.Path {
					case "/routers":
						fmt.Fprint(w, `{"routers":[{"id":"router","name":"fixture-router"}]}`)
					case "/ports":
						fmt.Fprintf(w, `{"ports":[{"id":"port","device_owner":"network:router_interface","fixed_ips":%s}]}`, ips)
					case "/ports/port":
						fmt.Fprintf(w, `{"port":{"id":"port","fixed_ips":%s}}`, ips)
					case "/networks":
						fmt.Fprint(w, `{"networks":[]}`)
					case "/subnets":
						fmt.Fprint(w, `{"subnets":[]}`)
					default:
						t.Errorf("unexpected %s %s", r.Method, r.URL)
						w.WriteHeader(404)
					}
				})
				m.routerView = routerview.New(m.client.Network, m.refreshInterval)
				loadViewData(m.routerView.Init(), func(msg tea.Msg) tea.Cmd { var cmd tea.Cmd; m.routerView, cmd = m.routerView.Update(msg); return cmd })
				for range 4 {
					if m.routerView.SelectedInterface() != nil {
						break
					}
					m.routerView, _ = m.routerView.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
				}
				if iface := m.routerView.SelectedInterface(); iface == nil || iface.SubnetID != "sub1" {
					t.Fatalf("fixture selected interface=%+v", iface)
				}
				executing = true
				_, cmd := m.executeAction(modal.ConfirmAction{Action: "remove_router_interface", ServerID: "router", Name: "fixture-router"})
				if cmd == nil {
					t.Fatal("missing removal command")
				}
				msg := cmd()
				if mutations != 1 {
					t.Fatalf("mutation requests=%d", mutations)
				}
				if failed {
					e, ok := msg.(shared.ResourceActionErrMsg)
					if !ok || e.Action != "Remove interface" || e.Err == nil {
						t.Fatalf("result=%#v", msg)
					}
				} else {
					s, ok := msg.(shared.ResourceActionMsg)
					if !ok || s.Action != "Removed interface from" || s.Name != "fixture-router" {
						t.Fatalf("result=%#v", msg)
					}
				}
			})
		}
	}
}

func TestImageActivationConfirmDispatch(t *testing.T) {
	for _, action := range []string{"deactivate_image", "reactivate_image"} {
		t.Run(action, func(t *testing.T) {
			calls := 0
			m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				want := "/images/id/actions/deactivate"
				if action == "reactivate_image" {
					want = "/images/id/actions/reactivate"
				}
				if r.Method != "POST" || r.URL.Path != want {
					t.Errorf("request=%s %s", r.Method, r.URL)
				}
				w.WriteHeader(204)
			})
			_, cmd := m.executeAction(modal.ConfirmAction{Action: action, ServerID: "id", Name: "name"})
			if cmd == nil {
				t.Fatal("no activation command")
			}
			if _, ok := cmd().(shared.ResourceActionMsg); !ok {
				t.Fatal("activation failed")
			}
			if calls != 1 {
				t.Errorf("requests=%d", calls)
			}
		})
	}
}
