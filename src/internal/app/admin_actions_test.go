package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/ui/serveradminact"
)

func TestAdminActionsAreAuditedAndReportedByRoot(t *testing.T) {
	cases := []struct {
		action, arg, body string
		want              audit.ActionType
	}{
		{"Migrate", "", `{"migrate":null}`, audit.ActionMigrate},
		{"Live Migrate", "host-2", `{"os-migrateLive":{"block_migration":false,"host":"host-2"}}`, audit.ActionLiveMigrate},
		{"Evacuate", "host-3", `{"evacuate":{"host":"host-3","onSharedStorage":false}}`, audit.ActionEvacuate},
		{"Force Delete", "", `{"forceDelete":""}`, audit.ActionForceDelete},
		{"Reset State", "error", `{"os-resetState":{"state":"error"}}`, audit.ActionResetState},
	}
	for _, tc := range cases {
		for _, failed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/error=%v", tc.action, failed), func(t *testing.T) {
				m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost || r.URL.Path != "/servers/srv/action" {
						t.Errorf("request %s %s", r.Method, r.URL)
					}
					body, _ := io.ReadAll(r.Body)
					checkJSON(t, body, tc.body)
					if failed {
						http.Error(w, "admin failure", http.StatusConflict)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					if tc.action == "Evacuate" {
						fmt.Fprint(w, `{}`)
						return
					}
					w.WriteHeader(http.StatusAccepted)
				})
				m.view = viewServerDetail
				m.serverDetail = detailFor("srv", "ACTIVE")
				// The modal has already closed when the request arrives.
				res, cmd := m.Update(serveradminact.ActionRequestMsg{Action: tc.action, ServerID: "srv", ServerName: "srv", Arg: tc.arg})
				m, _ = deliver(t, res.(Model), cmd)

				if failed {
					if m.activeModal != modalError || !strings.Contains(m.errModal.Context, tc.action) {
						t.Fatalf("failure not visible: modal=%v ctx=%q", m.activeModal, m.errModal.Context)
					}
				} else if !strings.Contains(m.statusBar.StickyHint, "✓") {
					t.Fatalf("success not visible: %q", m.statusBar.StickyHint)
				}
				if tc.action == "Force Delete" && !failed && m.view != viewServerList {
					t.Error("force-deleted server's detail view left open")
				}
				checkAudit(t, path, tc.want, "server", "srv", "srv", failed)
				entries, _ := audit.ReadEntries(path, 1)
				if tc.arg != "" {
					var details map[string]string
					if err := json.Unmarshal(entries[0].Details, &details); err != nil || !strings.Contains(fmt.Sprint(details), tc.arg) {
						t.Errorf("audit details=%s", entries[0].Details)
					}
				}
			})
		}
	}
}

func TestAdminActionRespectsInflightLock(t *testing.T) {
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) })
	req := serveradminact.ActionRequestMsg{Action: "Migrate", ServerID: "srv", ServerName: "srv"}
	res, first := m.Update(req)
	m = res.(Model)
	if first == nil {
		t.Fatal("not scheduled")
	}
	if _, dup := m.Update(req); dup != nil {
		t.Fatal("duplicate admin action scheduled while the first is pending")
	}
}

func TestEvacuatePasswordIsMaskedAndPartialResultWarns(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"password", `{"adminPass":"evac-secret"}`},
		{"malformed", `{"adminPass":123}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				posts++
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.body)
			})
			res, cmd := m.Update(serveradminact.ActionRequestMsg{Action: "Evacuate", ServerID: "srv", ServerName: "srv"})
			m, _ = deliver(t, res.(Model), cmd)
			if posts != 1 {
				t.Fatalf("evacuate requests=%d, must not be retried", posts)
			}
			hint := m.statusBar.StickyHint
			if strings.Contains(hint, "evac-secret") || !strings.Contains(hint, "✓ Evacuate srv") {
				t.Fatalf("status=%q", hint)
			}
			raw, _ := audit.ReadEntries(path, 5)
			if len(raw) != 1 || raw[0].Result != "success" || strings.Contains(fmt.Sprint(raw[0]), "evac-secret") {
				t.Fatalf("audit=%+v", raw)
			}
			if tc.name == "malformed" {
				if m.vmPassword.Active || !strings.Contains(hint, "password could not be read") {
					t.Fatalf("partial-result warning missing: %q", hint)
				}
				return
			}
			if !m.vmPassword.Active || strings.Contains(m.viewContent(), "evac-secret") {
				t.Fatal("password not offered through the masked modal")
			}
			res, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
			if !strings.Contains(res.(Model).viewContent(), "evac-secret") {
				t.Fatal("explicit reveal did not show the password")
			}
		})
	}
}
