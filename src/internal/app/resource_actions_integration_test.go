package app

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/modal"
)

func TestResourceConfirmActionsHTTPAndAudit(t *testing.T) {
	cases := []struct {
		action, path, resource string
		audit                  audit.ActionType
		code                   int
	}{
		{"delete_volume", "/volumes/id", "volume", audit.ActionDelete, 202},
		{"release_fip", "/floatingips/id", "floating_ip", audit.ActionDetachFIP, 204},
		{"disassociate_fip", "/floatingips/id", "floating_ip", audit.ActionDetachFIP, 200},
		{"delete_router", "/routers/id", "router", audit.ActionDeleteRouter, 204},
		{"delete_port", "/ports/id", "port", audit.ActionDeletePort, 204},
		{"delete_network", "/networks/id", "network", audit.ActionDeleteNet, 204},
		{"delete_subnet", "/subnets/id", "subnet", audit.ActionDeleteSubnet, 204},
		{"delete_sg", "/security-groups/id", "security_group", audit.ActionDeleteNet, 204},
		{"delete_sg_rule", "/security-group-rules/id", "security_group_rule", audit.ActionDeleteNet, 204},
		{"delete_lb", "/lbaas/loadbalancers/id", "load_balancer", audit.ActionDeleteLB, 204},
		{"delete_lb_listener", "/lbaas/listeners/id", "lb_listener", audit.ActionDeleteLB, 204},
		{"delete_lb_pool", "/lbaas/pools/id", "lb_pool", audit.ActionDeleteLB, 204},
		{"delete_lb_monitor", "/lbaas/healthmonitors/id", "lb_monitor", audit.ActionDeleteLB, 204},
		{"delete_lb_member", "/lbaas/pools/pool/members/id", "lb_member", audit.ActionDeleteLB, 204},
		{"delete_keypair", "/os-keypairs/id", "keypair", audit.ActionDeleteKey, 202},
		{"delete_image", "/images/id", "image", audit.ActionDeleteImage, 204},
	}
	for _, tc := range cases {
		for _, failed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/error=%v", tc.action, failed), func(t *testing.T) {
				calls := 0
				m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					method := "DELETE"
					if tc.action == "disassociate_fip" {
						method = "PUT"
						body, _ := io.ReadAll(r.Body)
						checkJSON(t, body, `{"floatingip":{"port_id":null}}`)
					}
					if r.URL.Path != tc.path || r.Method != method {
						t.Errorf("request=%s %s want=%s %s", r.Method, r.URL, method, tc.path)
					}
					if tc.action == "delete_lb" && r.URL.Query().Get("cascade") != "true" {
						t.Errorf("missing cascade: %s", r.URL)
					}
					w.Header().Set("Content-Type", "application/json")
					if failed {
						http.Error(w, "fixture failure", http.StatusConflict)
						return
					}
					w.WriteHeader(tc.code)
					if tc.action == "disassociate_fip" {
						fmt.Fprint(w, `{"floatingip":{"id":"id","port_id":null}}`)
					}
				})
				id, name := "id", "name"
				if tc.action == "delete_lb_member" {
					id = "pool|id"
				}
				if tc.action == "delete_keypair" {
					name = "id"
				}
				_, cmd := m.executeAction(modal.ConfirmAction{Action: tc.action, ServerID: id, Name: name})
				if cmd == nil {
					t.Fatal("no resource command")
				}
				msg := cmd()
				if calls != 1 {
					t.Fatalf("calls=%d", calls)
				}
				if failed {
					e, ok := msg.(shared.ResourceActionErrMsg)
					if !ok || e.Err == nil || !strings.Contains(e.Err.Error(), "fixture failure") || e.Name != name {
						t.Fatalf("result=%#v", msg)
					}
				} else {
					s, ok := msg.(shared.ResourceActionMsg)
					if !ok || s.Action == "" || s.Name != name {
						t.Fatalf("result=%#v", msg)
					}
				}
				checkAudit(t, path, tc.audit, tc.resource, "id", name, failed)
			})
		}
	}
}

func TestDetachVolumeHandlesLookupAttachmentAndPartialFailure(t *testing.T) {
	for _, mode := range []string{"lookup-error", "unattached", "success", "partial-failure"} {
		t.Run(mode, func(t *testing.T) {
			var detached []string
			m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/volumes/id":
					if mode == "lookup-error" {
						http.Error(w, "lookup failed", 404)
						return
					}
					if mode == "unattached" {
						fmt.Fprint(w, `{"volume":{"id":"id","status":"available","attachments":[]}}`)
						return
					}
					fmt.Fprint(w, `{"volume":{"id":"id","status":"in-use","attachments":[{"server_id":"s1"},{"server_id":"s2"}]}}`)
				case r.Method == "GET":
					fmt.Fprint(w, `{"volumeAttachments":[{"id":"attachment","volumeId":"id"}]}`)
				case r.Method == "DELETE":
					detached = append(detached, r.URL.Path)
					if mode == "partial-failure" && strings.Contains(r.URL.Path, "s1") {
						http.Error(w, "detach failed", http.StatusConflict)
						return
					}
					w.WriteHeader(202)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL)
				}
			})
			_, cmd := m.executeAction(modal.ConfirmAction{Action: "detach_volume", ServerID: "id", Name: "name"})
			msg := cmd()
			if mode == "success" {
				s, ok := msg.(shared.ResourceActionMsg)
				if !ok || s.Action != "Detached volume" {
					t.Fatalf("result=%#v", msg)
				}
			} else {
				e, ok := msg.(shared.ResourceActionErrMsg)
				if !ok || e.Err == nil {
					t.Fatalf("result=%#v", msg)
				}
				if mode == "unattached" && e.Err.Error() != "volume is not attached" {
					t.Errorf("error=%v", e.Err)
				}
			}
			if mode == "success" || mode == "partial-failure" {
				if len(detached) != 2 || detached[0] != "/servers/s1/os-volume_attachments/id" || detached[1] != "/servers/s2/os-volume_attachments/id" {
					t.Fatalf("detach requests=%v", detached)
				}
				checkAudit(t, path, audit.ActionDetachVolume, "volume", "id", "name", mode == "partial-failure")
			} else {
				entries, err := audit.ReadEntries(path, 10)
				if err != nil || len(entries) != 0 || len(detached) != 0 {
					t.Fatalf("early return audit=%v detached=%v err=%v", entries, detached, err)
				}
			}
		})
	}
}

func TestResourceBulkDispatchPrecedesServerGuard(t *testing.T) {
	for _, action := range []string{"delete_volumes_bulk", "delete_images_bulk", "detach_volumes_bulk"} {
		for _, failed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/error=%v", action, failed), func(t *testing.T) {
				var requests []string
				m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
					requests = append(requests, r.Method+" "+r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					if action == "detach_volumes_bulk" {
						if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/volumes/") {
							if failed && strings.HasSuffix(r.URL.Path, "bad") {
								http.Error(w, "lookup failed", 404)
								return
							}
							fmt.Fprint(w, `{"volume":{"status":"in-use","attachments":[{"server_id":"srv"}]}}`)
							return
						}
						if r.Method == "GET" {
							fmt.Fprint(w, `{"volumeAttachments":[]}`)
							return
						}
						if failed && strings.HasSuffix(r.URL.Path, "bad2") {
							http.Error(w, "detach failed", http.StatusConflict)
							return
						}
						w.WriteHeader(202)
						return
					}
					if strings.HasPrefix(r.URL.Path, "/servers/") {
						t.Errorf("resource dispatched to server endpoint %s", r.URL)
					}
					if failed && strings.Contains(r.URL.Path, "bad") {
						http.Error(w, "delete failed", http.StatusConflict)
						return
					}
					if action == "delete_images_bulk" {
						w.WriteHeader(204)
					} else {
						w.WriteHeader(202)
					}
				})
				refs := []modal.ServerRef{{ID: "bad", Name: "first"}, {ID: "good", Name: "middle"}, {ID: "bad2", Name: "last"}}
				_, cmd := m.executeAction(modal.ConfirmAction{Action: action, Servers: refs})
				if cmd == nil {
					t.Fatal("no bulk command")
				}
				r, ok := cmd().(bulkResultMsg)
				if !ok {
					t.Fatal("no structured bulk result")
				}
				if failed {
					if len(r.failed) != 2 || r.failed[0].ref.Name != "first" || r.failed[1].ref.Name != "last" || len(r.succeeded) != 1 || r.succeeded[0].Name != "middle" {
						t.Fatalf("aggregation=%+v", r)
					}
				} else if len(r.succeeded) != 3 || len(r.failed) != 0 {
					t.Fatalf("result=%+v", r)
				}
				if len(requests) < 3 {
					t.Fatalf("bulk stopped early: %v", requests)
				}
			})
		}
	}
}

func TestActionGuardsAndDisabledAudit(t *testing.T) {
	m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(202) })
	for _, action := range []modal.ConfirmAction{{Action: "unknown"}, {Action: "delete_lb_member", ServerID: "malformed"}, {Action: "remove_router_interface"}} {
		_, cmd := m.executeAction(action)
		if cmd != nil {
			t.Errorf("%s should not execute", action.Action)
		}
	}
	m.client.BlockStorage = nil
	m.client.Image = nil
	for _, action := range []string{"delete_volumes_bulk", "detach_volumes_bulk", "delete_images_bulk"} {
		_, cmd := m.executeAction(modal.ConfirmAction{Action: action, Servers: []modal.ServerRef{{ID: "id"}}})
		if cmd != nil {
			t.Errorf("%s missing service should not execute", action)
		}
	}
	m.auditLogger.SetEnabled(false)
	_, cmd := m.executeAction(modal.ConfirmAction{Action: "pause", ServerID: "id"})
	if _, ok := cmd().(shared.ServerActionMsg); !ok {
		t.Fatal("disabled audit blocked action")
	}
	entries, err := audit.ReadEntries(path, 10)
	if err != nil || len(entries) != 0 {
		t.Fatalf("disabled audit=%v err=%v", entries, err)
	}
	m.auditLogger = nil
	_, cmd = m.executeAction(modal.ConfirmAction{Action: "pause", ServerID: "id"})
	if _, ok := cmd().(shared.ServerActionMsg); !ok {
		t.Fatal("nil audit blocked action")
	}
}

func TestBulkDetachSkipsUnattachedVolume(t *testing.T) {
	requests := 0
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "GET" || r.URL.Path != "/volumes/id" {
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"volume":{"id":"id","status":"available","attachments":[]}}`)
	})
	_, cmd := m.executeAction(modal.ConfirmAction{Action: "detach_volumes_bulk", Servers: []modal.ServerRef{{ID: "id", Name: "name"}}})
	msg, ok := cmd().(bulkResultMsg)
	if !ok || msg.label != "Detach volumes" || len(msg.skipped) != 1 || len(msg.succeeded) != 0 || len(msg.failed) != 0 || requests != 1 {
		t.Fatalf("result=%+v requests=%d", msg, requests)
	}
}

func TestRescueWithoutReturnedPassword(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		t.Run(fmt.Sprint(bulk), func(t *testing.T) {
			m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{}`)
			})
			action := modal.ConfirmAction{Action: "rescue", ServerID: "id", Name: "name"}
			if bulk {
				action.Servers = []modal.ServerRef{{ID: "id", Name: "name"}}
			}
			_, cmd := m.executeAction(action)
			msg := cmd()
			if bulk {
				if r, ok := msg.(bulkResultMsg); !ok || r.label != "rescue" || len(r.succeeded) != 1 {
					t.Fatalf("result=%#v", msg)
				}
			} else if s, ok := msg.(shared.ServerActionMsg); !ok || s.Action != "Rescue" {
				t.Fatalf("result=%#v", msg)
			}
			checkAudit(t, path, audit.ActionRescue, "server", "id", "name", false)
		})
	}
}
