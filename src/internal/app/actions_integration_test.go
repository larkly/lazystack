package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/cloud"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/modal"
)

// Each fixture uses real gophercloud HTTP requests and an isolated on-disk audit log.
func actionFixture(t *testing.T, handler http.HandlerFunc) (Model, string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := &gophercloud.ServiceClient{ProviderClient: &gophercloud.ProviderClient{}, Endpoint: srv.URL + "/"}
	m := initTestModel()
	m.client = &cloud.Client{Compute: c, BlockStorage: c, Network: c, Image: c, LoadBalancer: c, DNS: c}
	m.cloudName, m.currentProjectID = "test-cloud", "test-project"
	path := filepath.Join(t.TempDir(), "audit.log")
	m.auditLogger = audit.NewLogger(path, true)
	return m, path
}

type serverActionCase struct {
	action, body, label string
	audit               audit.ActionType
}

var serverActionCases = []serverActionCase{
	{"delete", "", "Delete", audit.ActionDelete},
	{"soft reboot", `{"reboot":{"type":"SOFT"}}`, "Reboot", audit.ActionReboot},
	{"hard reboot", `{"reboot":{"type":"HARD"}}`, "Hard reboot", audit.ActionReboot},
	{"pause", `{"pause":null}`, "Pause", audit.ActionPause},
	{"unpause", `{"unpause":null}`, "Unpause", audit.ActionUnpause},
	{"suspend", `{"suspend":null}`, "Suspend", audit.ActionSuspend},
	{"resume", `{"resume":null}`, "Resume", audit.ActionResume},
	{"shelve", `{"shelve":null}`, "Shelve", audit.ActionShelve},
	{"unshelve", `{"unshelve":null}`, "Unshelve", audit.ActionUnshelve},
	{"stop", `{"os-stop":null}`, "Stop", audit.ActionStop},
	{"start", `{"os-start":null}`, "Start", audit.ActionStart},
	{"lock", `{"lock":null}`, "Lock", audit.ActionLock},
	{"unlock", `{"unlock":null}`, "Unlock", audit.ActionUnlock},
	{"rescue", `{"rescue":{}}`, "Rescue", audit.ActionRescue},
	{"unrescue", `{"unrescue":null}`, "Unrescue", audit.ActionUnrescue},
}

func checkJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Errorf("request JSON: %v (%s)", err, got)
		return
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("request body = %s, want %s", got, want)
	}
}

func checkAudit(t *testing.T, path string, action audit.ActionType, resource, id, name string, failed bool) {
	t.Helper()
	entries, err := audit.ReadEntries(path, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("audit entries=%+v err=%v", entries, err)
	}
	e := entries[0]
	result := "success"
	if failed {
		result = "error"
	}
	if e.Action != action || e.ResourceType != resource || e.ResourceID != id || e.ResourceName != name || e.Cloud != "test-cloud" || e.Project != "test-project" || e.Result != result || e.Timestamp.IsZero() {
		t.Errorf("audit = %+v", e)
	}
	if failed != (e.Error != "") {
		t.Errorf("audit error = %q, failed=%v", e.Error, failed)
	}
	if strings.Contains(e.Error, "fixture-password") {
		t.Error("password leaked to audit")
	}
}

func TestConfirmedServerActionsHTTPAndAudit(t *testing.T) {
	for _, tc := range serverActionCases {
		for _, failed := range []bool{false, true} {
			for _, bulk := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/error=%v/bulk=%v", tc.action, failed, bulk), func(t *testing.T) {
					calls := 0
					m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
						calls++
						wantPath, wantMethod := "/servers/id/action", "POST"
						if tc.action == "delete" {
							wantPath, wantMethod = "/servers/id", "DELETE"
						}
						if r.URL.Path != wantPath || r.Method != wantMethod {
							t.Errorf("request %s %s, want %s %s", r.Method, r.URL, wantMethod, wantPath)
						}
						body, _ := io.ReadAll(r.Body)
						if tc.body != "" {
							checkJSON(t, body, tc.body)
						}
						w.Header().Set("Content-Type", "application/json")
						if failed {
							w.WriteHeader(409)
							fmt.Fprint(w, `{"conflictingRequest":{"message":"fixture failure"}}`)
							return
						}
						if tc.action == "rescue" {
							fmt.Fprint(w, `{"adminPass":"fixture-password"}`)
							return
						}
						if tc.action == "delete" {
							w.WriteHeader(204)
						} else {
							w.WriteHeader(202)
						}
					})
					action := modal.ConfirmAction{Confirm: true, Action: tc.action, ServerID: "id", Name: "name"}
					if bulk {
						action.Servers = []modal.ServerRef{{ID: "id", Name: "name"}}
					}
					m.activeModal = modalConfirm
					next, cmd := m.Update(action)
					if next.(Model).activeModal != modalNone || cmd == nil {
						t.Fatal("confirmation did not dismiss modal and schedule action")
					}
					if calls != 0 {
						t.Fatal("action ran before command execution")
					}
					msg := unwrapResult(cmd())
					if calls != 1 {
						t.Fatalf("HTTP calls=%d", calls)
					}
					if bulk {
						if c, ok := msg.(credentialsMsg); ok && tc.action == "rescue" && !failed {
							if len(c.creds) != 1 || c.creds[0].Server != "name" || c.creds[0].Secret != "fixture-password" {
								t.Fatalf("credentials=%#v", c)
							}
							msg = c.result
						}
						r, ok := msg.(bulkResultMsg)
						if !ok || r.label != tc.action || r.noun != "servers" {
							t.Fatalf("result=%#v", msg)
						}
						if failed {
							if len(r.failed) != 1 || r.failed[0].ref.ID != "id" || !strings.Contains(r.failed[0].err.Error(), "fixture failure") {
								t.Fatalf("failed=%+v", r.failed)
							}
						} else if len(r.succeeded) != 1 || r.succeeded[0].ID != "id" || len(r.failed) != 0 {
							t.Fatalf("result=%+v", r)
						}
					} else if failed {
						e, ok := msg.(shared.ServerActionErrMsg)
						if !ok || e.Err == nil || !strings.Contains(e.Err.Error(), "fixture failure") {
							t.Fatalf("result=%#v", msg)
						}
					} else if tc.action == "delete" {
						if d, ok := msg.(serverDeletedMsg); !ok || d.id != "id" || d.name != "name" {
							t.Fatalf("result=%#v", msg)
						}
					} else {
						if c, ok := msg.(credentialsMsg); ok {
							if tc.action != "rescue" || len(c.creds) != 1 || c.creds[0].Server != "name" || c.creds[0].Secret != "fixture-password" {
								t.Fatalf("credentials=%#v", c)
							}
							msg = c.result
						} else if tc.action == "rescue" {
							t.Fatalf("rescue password not delivered: %#v", msg)
						}
						s, ok := msg.(shared.ServerActionMsg)
						if !ok {
							t.Fatalf("result=%#v", msg)
						}
						if s.Action != tc.label || s.Name != "name" {
							t.Errorf("result=%+v want %s/name", s, tc.label)
						}
					}
					checkAudit(t, path, tc.audit, "server", "id", "name", failed)
				})
			}
		}
	}
}

func TestCancelledConfirmDoesNotExecuteOrAudit(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		t.Run(fmt.Sprint(bulk), func(t *testing.T) {
			m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Errorf("unexpected request %s", r.URL) })
			m.activeModal = modalConfirm
			a := modal.ConfirmAction{Action: "delete", ServerID: "id", Name: "name", Confirm: false}
			if bulk {
				a.Servers = []modal.ServerRef{{ID: "id", Name: "name"}}
			}
			next, cmd := m.Update(a)
			if cmd != nil || next.(Model).activeModal != modalNone {
				t.Fatal("cancel should only close modal")
			}
			entries, err := audit.ReadEntries(path, 10)
			if err != nil || len(entries) != 0 {
				t.Fatalf("cancel audit=%v err=%v", entries, err)
			}
		})
	}
}

func TestBulkMixedActionsAggregateErrorsAndContinue(t *testing.T) {
	var requested []string
	m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(r.URL.Path, "good") {
			checkJSON(t, body, `{"os-start":null}`)
			w.WriteHeader(202)
			return
		}
		checkJSON(t, body, `{"os-stop":null}`)
		http.Error(w, "cannot stop "+r.URL.Path, 409)
	})
	_, cmd := m.executeAction(modal.ConfirmAction{Action: "stop", Servers: []modal.ServerRef{{ID: "bad1", Name: "first"}, {ID: "good", Name: "middle", Action: "start"}, {ID: "bad2", Name: "last"}}})
	msg, ok := cmd().(bulkResultMsg)
	if !ok || msg.label != "mixed action (start:1, stop:2)" || msg.summary() != "mixed action (start:1, stop:2): 1 of 3 servers succeeded, 2 failed" {
		t.Fatalf("result=%+v", msg)
	}
	if len(msg.failed) != 2 || msg.failed[0].ref.Name != "first" || msg.failed[0].ref.Action != "stop" || msg.failed[1].ref.Name != "last" ||
		len(msg.succeeded) != 1 || msg.succeeded[0].Name != "middle" || msg.succeeded[0].Action != "start" {
		t.Fatalf("aggregation=%+v", msg)
	}
	if !strings.Contains(describeBulkItem(msg.failed[0]), "first (bad1) [stop]: ") {
		t.Fatalf("failure line=%q", describeBulkItem(msg.failed[0]))
	}
	if !reflect.DeepEqual(requested, []string{"/servers/bad1/action", "/servers/good/action", "/servers/bad2/action"}) {
		t.Fatalf("requests=%v", requested)
	}
	entries, err := audit.ReadEntries(path, 10)
	if err != nil || len(entries) != 3 {
		t.Fatalf("audit=%v err=%v", entries, err)
	}
	byID := map[string]audit.Entry{}
	for _, e := range entries {
		byID[e.ResourceID] = e
	}
	for _, id := range []string{"bad1", "bad2"} {
		if e := byID[id]; e.Result != "error" || e.Action != audit.ActionStop || e.Error == "" {
			t.Errorf("audit=%+v", e)
		}
	}
	if e := byID["good"]; e.Result != "success" || e.Action != audit.ActionStart || e.Error != "" {
		t.Errorf("audit=%+v", e)
	}
}
