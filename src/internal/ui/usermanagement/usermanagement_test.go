package usermanagement

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateNavigationConfirmationAndViews(t *testing.T) {
	m := New(nil, gophercloud.EndpointOpts{})
	m.SetSize(150, 8)
	m.cursor = 9
	m.confirmingDelete = "old"
	m.toggling = "old"
	m.err = "old error"
	m, _ = m.Update(usersLoadedMsg{items: []compute.User{{ID: "a", Name: "Alice", Email: "alice@example.test", Enabled: true}, {ID: "b", Name: "Bob"}, {ID: "c", Name: "Carol"}}})
	if m.loading || m.cursor != 2 || m.err != "" || m.confirmingDelete != "" || m.toggling != "" {
		t.Fatal("load did not clear operation state and clamp")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	if m.cursor != 0 {
		t.Fatal("page up")
	}
	if !strings.Contains(m.View(), "Alice") || !strings.Contains(m.View(), "Yes") {
		t.Fatal("user fields missing")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.cursor != 1 {
		t.Fatal("down")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	if m.confirmingDelete != "b" || !strings.Contains(m.View(), "Really delete user") {
		t.Fatal("delete not confirmed first")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.cursor != 1 {
		t.Fatal("navigation escaped confirmation")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	if m.confirmingDelete != "" {
		t.Fatal("cancel delete")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'y', Text: "y"}))
	if cmd == nil || m.confirmingDelete != "" {
		t.Fatal("confirm delete command")
	}
	m, _ = m.Update(usersErrMsg{err: errors.New("permission denied")})
	if m.loading || !strings.Contains(m.View(), "permission denied") {
		t.Fatal("error state")
	}
	m, _ = m.Update(usersLoadedMsg{})
	if !strings.Contains(m.View(), "No users found") {
		t.Fatal("empty view")
	}
	m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if cmd == nil || cmd().(shared.ViewChangeMsg).View != "serverlist" {
		t.Fatal("back navigation")
	}
	if m.ForceRefresh() == nil || !m.loading {
		t.Fatal("refresh")
	}
}

func TestUserOperationsHTTP(t *testing.T) {
	for _, operation := range []string{"fetch", "toggle", "delete"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fail=%v", operation, fail), func(t *testing.T) {
				methods := []string{}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					methods = append(methods, r.Method)
					w.Header().Set("Content-Type", "application/json")
					if fail {
						http.Error(w, "denied", http.StatusForbidden)
						return
					}
					switch r.Method {
					case http.MethodGet:
						if r.URL.Path != "/v3/users" {
							t.Errorf("path %s", r.URL.Path)
						}
						fmt.Fprint(w, `{"users":[{"id":"u","name":"Alice","enabled":false}]}`)
					case http.MethodPatch:
						if r.URL.Path != "/v3/users/u" {
							t.Errorf("path %s", r.URL.Path)
						}
						var body struct {
							User struct {
								Enabled *bool `json:"enabled"`
							} `json:"user"`
						}
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.User.Enabled == nil || *body.User.Enabled {
							t.Error("toggle must explicitly disable enabled user")
						}
						fmt.Fprint(w, `{"user":{"id":"u","enabled":false}}`)
					case http.MethodDelete:
						if r.URL.Path != "/v3/users/u" {
							t.Errorf("path %s", r.URL.Path)
						}
						w.WriteHeader(204)
					default:
						t.Errorf("method %s", r.Method)
					}
				}))
				defer server.Close()
				pc := &gophercloud.ProviderClient{IdentityEndpoint: server.URL + "/", EndpointLocator: func(gophercloud.EndpointOpts) (string, error) { return server.URL + "/", nil }}
				m := New(pc, gophercloud.EndpointOpts{Region: "test-region"})
				cmd := m.fetch()
				if operation == "toggle" {
					cmd = m.doToggle(compute.User{ID: "u", Enabled: true})
				}
				if operation == "delete" {
					cmd = m.doDelete("u")
				}
				msg := cmd()
				if e, ok := msg.(usersErrMsg); ok {
					t.Logf("operation error: %v", e.err)
				}
				if fail {
					if _, ok := msg.(usersErrMsg); !ok || len(methods) != 1 {
						t.Fatalf("want error without follow-up: %#v %v", msg, methods)
					}
					return
				}
				got, ok := msg.(usersLoadedMsg)
				if !ok || len(got.items) != 1 || got.items[0].Name != "Alice" || got.items[0].Enabled {
					t.Fatalf("result %#v", msg)
				}
				want := "GET"
				if operation == "toggle" {
					want = "PATCH,GET"
				}
				if operation == "delete" {
					want = "DELETE,GET"
				}
				if strings.Join(methods, ",") != want {
					t.Fatalf("methods=%v want=%s", methods, want)
				}
			})
		}
	}
}
