package usermanagement

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
)

func press(m Model, k tea.KeyPressMsg) (Model, tea.Cmd) { return m.Update(k) }

func text(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// keystone serves the identity calls user management makes and counts
// every mutating request.
func keystone(t *testing.T, listStatus int) (*gophercloud.ProviderClient, *atomic.Int32) {
	t.Helper()
	var mutations atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if listStatus != http.StatusOK {
				http.Error(w, "list broken", listStatus)
				return
			}
			fmt.Fprint(w, `{"users":[{"id":"a","name":"Alice","enabled":false},{"id":"b","name":"Bob","enabled":true}]}`)
		case http.MethodPatch:
			mutations.Add(1)
			fmt.Fprint(w, `{"user":{"id":"a","enabled":false}}`)
		case http.MethodDelete:
			mutations.Add(1)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	pc := &gophercloud.ProviderClient{IdentityEndpoint: srv.URL + "/", EndpointLocator: func(gophercloud.EndpointOpts) (string, error) { return srv.URL + "/", nil }}
	return pc, &mutations
}

func loaded(pc *gophercloud.ProviderClient) Model {
	// A region makes NewIdentityV3 use the stub's EndpointLocator.
	m := New(pc, gophercloud.EndpointOpts{Region: "test-region"})
	m.SetSize(200, 20)
	m, _ = m.Update(usersLoadedMsg{items: []compute.User{{ID: "a", Name: "Alice", Enabled: true}, {ID: "b", Name: "Bob", Enabled: true}}})
	return m
}

func TestToggleRequiresConfirmationAndEscapeSendsNothing(t *testing.T) {
	pc, mutations := keystone(t, http.StatusOK)
	m := loaded(pc)

	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("enter mutated without confirmation")
	}
	if !strings.Contains(m.View(), "Really disable user Alice (a)? y confirm • n cancel") {
		t.Fatalf("confirmation prompt missing identity/keys:\n%s", m.View())
	}
	m, cmd = press(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.pending != nil || cmd != nil {
		t.Fatal("escape did not simply cancel the confirmation")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if mutations.Load() != 0 {
		t.Fatalf("%d requests sent without confirmation", mutations.Load())
	}

	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, cmd = press(m, text('y'))
	if cmd == nil {
		t.Fatal("confirmed toggle did not run")
	}
	for _, msg := range runAll(cmd) {
		m, _ = m.Update(msg)
	}
	if mutations.Load() != 1 || !strings.Contains(m.notice, "Disabled Alice (a)") {
		t.Fatalf("mutations=%d notice=%q", mutations.Load(), m.notice)
	}
}

func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runAll(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestConfirmationHonoursReboundKeys(t *testing.T) {
	prev := shared.Keys
	t.Cleanup(func() { shared.Keys = prev })
	shared.Keys.Confirm = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "confirm"))
	shared.Keys.Deny = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "cancel"))
	shared.Keys.Delete = key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "delete"))

	pc, mutations := keystone(t, http.StatusOK)
	m := loaded(pc)
	for _, k := range []tea.KeyPressMsg{text('d'), {Code: 'd', Mod: tea.ModCtrl}} {
		if m, _ = press(m, k); m.pending != nil {
			t.Fatalf("old delete key %q still starts a delete", k.String())
		}
	}
	m, _ = press(m, text('X'))
	if m.pending == nil || m.pending.kind != actionDelete {
		t.Fatal("configured delete key did not ask for confirmation")
	}
	if v := m.View(); !strings.Contains(v, "Really delete user Alice (a)? o confirm • x cancel") {
		t.Fatalf("prompt does not show configured keys:\n%s", v)
	}
	for _, r := range []rune{'y', 'n'} {
		var cmd tea.Cmd
		if m, cmd = press(m, text(r)); cmd != nil || m.pending == nil {
			t.Fatalf("old key %q acted on the confirmation", r)
		}
	}
	m, _ = press(m, text('x'))
	if m.pending != nil {
		t.Fatal("configured deny key did not cancel")
	}
	m, _ = press(m, text('X'))
	m, cmd := press(m, text('o'))
	if cmd == nil {
		t.Fatal("configured confirm key did not run the delete")
	}
	for _, msg := range runAll(cmd) {
		m, _ = m.Update(msg)
	}
	if mutations.Load() != 1 {
		t.Fatalf("mutations = %d, want 1", mutations.Load())
	}
	if !strings.Contains(m.Hints(), "X delete") {
		t.Errorf("hints do not show the configured delete key: %q", m.Hints())
	}
}

func authenticatedAs(t *testing.T, pc *gophercloud.ProviderClient, userID string) {
	t.Helper()
	var r tokens.CreateResult
	r.Body = map[string]any{"token": map[string]any{"user": map[string]any{"id": userID, "name": "Alice"}}}
	if err := pc.SetTokenAndAuthResult(r); err != nil {
		t.Fatal(err)
	}
}

func TestRefusesToDisableOrDeleteCurrentUser(t *testing.T) {
	pc, mutations := keystone(t, http.StatusOK)
	authenticatedAs(t, pc, "a")
	m := loaded(pc)
	if got := m.currentUserID(); got != "a" {
		t.Fatalf("currentUserID = %q", got)
	}
	for _, k := range []tea.KeyPressMsg{{Code: tea.KeyEnter}, {Code: 'd', Mod: tea.ModCtrl}} {
		var cmd tea.Cmd
		m, cmd = press(m, k)
		if m.pending != nil || cmd != nil {
			t.Fatalf("%s on the current user was not refused", k.String())
		}
		if !strings.Contains(m.View(), "authenticated as") {
			t.Fatalf("no explanation shown:\n%s", m.View())
		}
	}
	// Other users are still manageable.
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.pending == nil || m.pending.user.ID != "b" {
		t.Fatal("other users can no longer be toggled")
	}
	if mutations.Load() != 0 {
		t.Fatal("refused action reached Keystone")
	}
}

func TestMutationSuccessReportedWhenRefreshFails(t *testing.T) {
	pc, mutations := keystone(t, http.StatusInternalServerError)
	m := loaded(pc)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, cmd := press(m, text('y'))
	for _, msg := range runAll(cmd) {
		m, _ = m.Update(msg)
	}
	if mutations.Load() != 1 {
		t.Fatalf("mutations = %d", mutations.Load())
	}
	v := m.View()
	if !strings.Contains(v, "Disabled Alice (a).") || !strings.Contains(v, "Refreshing the user list failed") {
		t.Fatalf("success hidden by refresh failure:\n%s", v)
	}
	if len(m.items) != 2 || m.items[0].Enabled {
		t.Fatalf("list not updated locally: %+v", m.items)
	}
}
