package serverdetail

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/testutil"
)

// detailBackend answers every detail request for any server ID, echoing the
// ID so stale data is recognisable. Status 500 makes every request fail.
func detailBackend(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case len(parts) == 2 && parts[0] == "servers":
			fmt.Fprintf(w, `{"server":{"id":%q,"name":"name-%s","status":"ACTIVE","flavor":{"id":"f"},"os-extended-volumes:volumes_attached":[{"id":"vol-%s"}]}}`, parts[1], parts[1], parts[1])
		case strings.HasSuffix(r.URL.Path, "/action"):
			fmt.Fprintf(w, `{"output":"console of %s"}`, parts[1])
		case strings.HasSuffix(r.URL.Path, "/os-instance-actions"):
			fmt.Fprintf(w, `{"instanceActions":[{"action":"reboot","request_id":"req-%s"}]}`, parts[1])
		case strings.HasSuffix(r.URL.Path, "/ports"):
			fmt.Fprintf(w, `{"ports":[{"id":"port-%s","device_id":%q}]}`, r.URL.Query().Get("device_id"), r.URL.Query().Get("device_id"))
		case len(parts) == 2 && parts[0] == "volumes":
			fmt.Fprintf(w, `{"volume":{"id":%q,"name":"stale","status":"in-use"}}`, parts[1])
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// replies runs every data fetch of m and returns the resulting messages.
func replies(t *testing.T, m Model) []tea.Msg {
	t.Helper()
	cmds := []tea.Cmd{m.fetchServer(), m.fetchConsole(), m.fetchActions(), m.fetchInterfaces(),
		m.fetchVolumeInfo([]compute.VolumeAttachment{{ID: "vol-" + m.serverID}})}
	var out []tea.Msg
	for _, c := range cmds {
		out = append(out, c())
	}
	return out
}

func snapshot(m Model) Model {
	m.spinner = Model{}.spinner
	return m
}

func TestLateRepliesForAnotherServerAreIgnored(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client, cleanup := testutil.FakeServiceClient(detailBackend(status))
			t.Cleanup(cleanup)
			a := New(client, client, client, "A", time.Hour)
			stale := replies(t, a)

			b := New(client, client, client, "B", time.Hour)
			b.SetServer(&compute.Server{ID: "B", Name: "bravo", Status: "ACTIVE"})
			before := snapshot(b)
			for _, msg := range stale {
				next, cmd := b.Update(msg)
				if cmd != nil {
					t.Errorf("%T for A made B issue a command", msg)
				}
				if !reflect.DeepEqual(snapshot(next), before) {
					t.Fatalf("%T for A changed B's state", msg)
				}
				b = next
			}
			if b.ServerID() != "B" || b.Server().ID != "B" {
				t.Fatal("B's identity changed")
			}
		})
	}
}

func TestRepliesForSameServerIDFromOlderModelAreIgnored(t *testing.T) {
	client, cleanup := testutil.FakeServiceClient(detailBackend(http.StatusOK))
	t.Cleanup(cleanup)
	// Same server ID opened again (for example after a project switch):
	// replies from the earlier model instance must not apply.
	older := New(client, client, client, "S", time.Hour)
	stale := replies(t, older)
	current := New(client, client, client, "S", time.Hour)
	for _, msg := range stale {
		next, _ := current.Update(msg)
		if next.server != nil || len(next.consoleLines) != 0 || len(next.actions) != 0 || len(next.interfaces) != 0 || len(next.volumeInfo) != 0 {
			t.Fatalf("%T from an older model instance was applied", msg)
		}
	}
	// Its own replies are applied.
	for _, msg := range replies(t, current) {
		current, _ = current.Update(msg)
	}
	if current.server == nil || current.server.ID != "S" || len(current.actions) == 0 {
		t.Fatal("the model's own replies were not applied")
	}
}

func TestSetServerRejectsAnotherServer(t *testing.T) {
	m := New(nil, nil, nil, "B", time.Hour)
	m.SetServer(&compute.Server{ID: "B", Name: "bravo"})
	m.SetServer(&compute.Server{ID: "A", Name: "alpha"})
	if m.Server().ID != "B" || m.ServerName() != "bravo" {
		t.Fatalf("SetServer accepted another server: %+v", m.Server())
	}
}
