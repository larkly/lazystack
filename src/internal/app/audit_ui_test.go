package app

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/shared"
)

// uiResult stands in for a UI package's command result message.
type uiResult struct {
	shared.Audit
}

func readAudit(t *testing.T, path string) []audit.Entry {
	t.Helper()
	entries, err := audit.ReadEntries(path, 100)
	if err != nil && !strings.Contains(err.Error(), "no such file") {
		t.Fatal(err)
	}
	return entries
}

// runAll executes cmd, expanding batches, and returns the leaf messages.
func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, runAll(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestUIResultsAreAudited(t *testing.T) {
	boom := errors.New("quota exceeded")
	create := func(err error) shared.Audit {
		return shared.NewAudit(audit.ActionCreateNet, "network", "net-1", "web", err)
	}
	result := func(msg tea.Msg) tea.Cmd { return func() tea.Msg { return msg } }
	type want struct {
		action       audit.ActionType
		id, result   string
		errSubstring string
	}
	cases := []struct {
		name string
		cmd  tea.Cmd
		want []want
	}{
		{"success", result(uiResult{create(nil)}), []want{{audit.ActionCreateNet, "net-1", "success", ""}}},
		{"failure", result(uiResult{create(boom)}), []want{{audit.ActionCreateNet, "net-1", "error", "quota exceeded"}}},
		{"several records", result(uiResult{shared.Audits(
			shared.AuditRecord{Action: audit.ActionAllocateFIP, ResourceType: "floating_ip", ResourceID: "fip-1"},
			shared.AuditRecord{Action: audit.ActionAttachFIP, ResourceType: "floating_ip", ResourceID: "fip-1", Err: boom},
		)}), []want{{audit.ActionAllocateFIP, "fip-1", "success", ""}, {audit.ActionAttachFIP, "fip-1", "error", "quota"}}},
		{"inside a tracked action result", result(actionResultMsg{msg: uiResult{create(nil)}}),
			[]want{{audit.ActionCreateNet, "net-1", "success", ""}}},
		{"inside a connection envelope", result(connScopedMsg{gen: 1, msg: uiResult{create(nil)}}),
			[]want{{audit.ActionCreateNet, "net-1", "success", ""}}},
		{"in a batch", tea.Batch(result(uiResult{create(nil)}), result(uiResult{create(boom)})),
			[]want{{audit.ActionCreateNet, "net-1", "success", ""}, {audit.ActionCreateNet, "net-1", "error", "quota"}}},
		{"no record", result(uiResult{}), nil},
		{"root model result", result(shared.ResourceActionMsg{Action: "Released", Name: "x"}), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {})
			runAll(m.auditResults(tc.cmd))
			got := readAudit(t, path)
			if len(got) != len(tc.want) {
				t.Fatalf("entries = %+v, want %d", got, len(tc.want))
			}
			for _, w := range tc.want {
				found := false
				for _, e := range got {
					if e.Action == w.action && e.ResourceID == w.id && e.Result == w.result &&
						strings.Contains(e.Error, w.errSubstring) && e.Cloud == "test-cloud" && e.Project == "test-project" {
						found = true
					}
				}
				if !found {
					t.Errorf("missing %+v in %+v", w, got)
				}
			}
		})
	}
}

// A result that passes several command wrappers (nested Updates) is
// recorded once.
func TestUIResultAuditedOnce(t *testing.T) {
	m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {})
	msg := uiResult{shared.NewAudit(audit.ActionRename, "server", "srv-1", "web", nil)}
	inner := m.auditResults(func() tea.Msg { return msg })
	runAll(m.auditResults(inner))
	runAll(m.auditResults(func() tea.Msg { return msg })) // redelivered copy
	if got := readAudit(t, path); len(got) != 1 {
		t.Fatalf("entries = %+v, want exactly one", got)
	}
}

// A form's mutation is audited under the connection that issued it, even
// when the user switched clouds before the result arrived and the result
// itself is therefore dropped.
func TestUIMutationAuditedAcrossConnectionSwitch(t *testing.T) {
	m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/networks") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"network":{"id":"net-9","name":"web","status":"ACTIVE"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	m.connGen = 1
	m, _ = m.openNetworkCreate()
	for _, r := range "web" {
		res, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = res.(Model)
	}
	res, submit := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m = res.(Model)
	if submit == nil {
		t.Fatal("network create not submitted")
	}

	// The user switches to another cloud while the request runs.
	m.connGen = 2
	m.cloudName, m.currentProjectID = "other-cloud", "other-project"

	for _, msg := range runAll(submit) {
		res, _ = m.Update(msg)
		m = res.(Model)
	}
	entries := readAudit(t, path)
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want the network create", entries)
	}
	e := entries[0]
	if e.Action != audit.ActionCreateNet || e.ResourceID != "net-9" || e.ResourceName != "web" ||
		e.Result != "success" || e.Cloud != "test-cloud" || e.Project != "test-project" {
		t.Fatalf("entry = %+v", e)
	}
}
