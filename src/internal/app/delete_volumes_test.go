package app

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/modal"
)

// stubDetachWait replaces the detach-wait sleep with a recorder so the
// retry/exhaustion paths run without wall-clock delays.
func stubDetachWait(t *testing.T) *[]time.Duration {
	t.Helper()
	slept := new([]time.Duration)
	orig := sleepCtx
	sleepCtx = func(ctx context.Context, d time.Duration) error {
		*slept = append(*slept, d)
		return ctx.Err()
	}
	t.Cleanup(func() { sleepCtx = orig })
	return slept
}

// runBounded executes a command but fails the test instead of hanging the
// suite if it never completes.
func runBounded(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("nil command")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		return msg
	case <-time.After(20 * time.Second):
		t.Fatal("command did not complete")
		return nil
	}
}

func auditByResource(t *testing.T, path string) map[string]audit.Entry {
	t.Helper()
	entries, err := audit.ReadEntries(path, 50)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]audit.Entry{}
	for _, e := range entries {
		out[e.ResourceType+"/"+e.ResourceID] = e
	}
	return out
}

func TestDeleteWithVolumesWaitsBeforeDeleting(t *testing.T) {
	attempts, interval := volumeDetachPollAttempts, volumeDetachPollInterval
	for _, mode := range []string{"available", "retry", "exhausted", "lookup-error", "lookup-recovers", "volume-errors", "server-error", "no-block-storage", "keep-volumes"} {
		t.Run(mode, func(t *testing.T) {
			slept := stubDetachWait(t)
			var requests []string
			polls := 0
			m, path := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.Method+" "+r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == "GET" && strings.Contains(r.URL.Path, "os-volume_attachments"):
					fmt.Fprint(w, `{"volumeAttachments":[{"id":"vol","volumeId":"vol"}]}`)
				case r.Method == "GET":
					polls++
					if mode == "lookup-error" || (mode == "lookup-recovers" && polls == 1) {
						http.Error(w, "lookup failed", http.StatusInternalServerError)
						return
					}
					status := "available"
					if mode == "exhausted" || (mode == "retry" && polls == 1) {
						status = "in-use"
					}
					fmt.Fprintf(w, `{"volume":{"id":"vol","status":%q}}`, status)
				case strings.Contains(r.URL.Path, "os-volume_attachments"):
					if mode == "volume-errors" {
						http.Error(w, "detach failed", http.StatusConflict)
						return
					}
					w.WriteHeader(http.StatusAccepted)
				case r.URL.Path == "/servers/id":
					if mode == "server-error" {
						http.Error(w, "server failed", http.StatusConflict)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				case r.URL.Path == "/volumes/vol":
					if mode == "volume-errors" {
						http.Error(w, "delete failed", http.StatusConflict)
						return
					}
					w.WriteHeader(http.StatusAccepted)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			})
			if mode == "no-block-storage" {
				m.client.BlockStorage = nil
			}
			_, cmd := m.executeAction(modal.ConfirmAction{Action: "delete", ServerID: "id", Name: "name", DeleteVolumes: mode != "keep-volumes", VolumeIDs: []string{"vol"}})
			msg := runBounded(t, cmd)

			wantPolls := 1
			switch mode {
			case "retry", "lookup-recovers":
				wantPolls = 2
			case "exhausted", "lookup-error":
				wantPolls = attempts
			case "no-block-storage", "keep-volumes":
				wantPolls = 0
			}
			wantSleeps := make([]time.Duration, 0)
			for range max(wantPolls-1, 0) {
				wantSleeps = append(wantSleeps, interval)
			}
			if polls != wantPolls || !slices.Equal(*slept, wantSleeps) {
				t.Errorf("polls=%d want=%d sleeps=%v want=%v", polls, wantPolls, *slept, wantSleeps)
			}
			// A volume whose detachment was never confirmed must not be deleted.
			safe := mode == "available" || mode == "retry" || mode == "lookup-recovers" || mode == "volume-errors"
			want := []string{}
			if wantPolls > 0 {
				want = append(want, "GET /servers/id/os-volume_attachments", "DELETE /servers/id/os-volume_attachments/vol")
				for range wantPolls {
					want = append(want, "GET /volumes/vol")
				}
			}
			want = append(want, "DELETE /servers/id")
			if safe && mode != "server-error" {
				want = append(want, "DELETE /volumes/vol")
			}
			if !reflect.DeepEqual(requests, want) {
				t.Errorf("request order=%v want=%v", requests, want)
			}

			byRes := auditByResource(t, path)
			if mode == "server-error" {
				if _, ok := msg.(shared.ServerActionErrMsg); !ok {
					t.Fatalf("result=%#v", msg)
				}
				if e := byRes["server/id"]; e.Result != "error" || e.Action != audit.ActionDelete {
					t.Errorf("server audit=%+v", e)
				}
				return
			}
			d, ok := msg.(serverDeletedMsg)
			if !ok || d.id != "id" || d.name != "name" {
				t.Fatalf("result=%#v", msg)
			}
			if e := byRes["server/id"]; e.Result != "success" || e.Action != audit.ActionDelete || e.Cloud != "test-cloud" || e.Project != "test-project" {
				t.Errorf("server audit=%+v", e)
			}
			vol := byRes["volume/vol"]
			switch mode {
			case "no-block-storage", "keep-volumes":
				if len(d.volumeFailures) != 0 || len(d.volumesDeleted) != 0 || vol.ResourceID != "" {
					t.Errorf("unexpected volume outcome %+v audit=%+v", d, vol)
				}
			case "available", "retry", "lookup-recovers":
				if len(d.volumeFailures) != 0 || !reflect.DeepEqual(d.volumesDeleted, []string{"vol"}) || vol.Result != "success" {
					t.Errorf("outcome=%+v audit=%+v", d, vol)
				}
			default:
				wantErr := map[string]string{"exhausted": "in-use", "lookup-error": "lookup failed", "volume-errors": "delete failed"}[mode]
				if len(d.volumeFailures) != 1 || d.volumeFailures[0].volumeID != "vol" || !strings.Contains(d.volumeFailures[0].err.Error(), wantErr) || len(d.volumesDeleted) != 0 {
					t.Fatalf("outcome=%+v want failure containing %q", d, wantErr)
				}
				if vol.Result == "success" || vol.Error == "" {
					t.Errorf("volume audit=%+v", vol)
				}
			}
		})
	}
}

// Deleting a server whose volume cleanup partly failed must still leave the
// detail view of the deleted server and list every failed volume.
func TestServerDeletedWithVolumeFailuresNavigatesAndReports(t *testing.T) {
	m := initTestModel()
	m.view = viewServerDetail
	m.serverDetail = testServerDetailWithServer("id", "name")
	res, cmd := m.Update(serverDeletedMsg{id: "id", name: "name", volumeFailures: []volumeCleanupFailure{
		{volumeID: "vol-a", err: fmt.Errorf("delete: boom")},
		{volumeID: "vol-b", err: fmt.Errorf("not deleted: detachment not confirmed")},
	}})
	got := res.(Model)
	if got.view != viewServerList || got.statusBar.CurrentView != "serverlist" {
		t.Fatalf("view=%v status=%q", got.view, got.statusBar.CurrentView)
	}
	if !strings.Contains(got.statusBar.StickyHint, "2 volume") {
		t.Errorf("status hint=%q", got.statusBar.StickyHint)
	}
	if got.activeModal != modalError {
		t.Fatal("volume failures not shown")
	}
	text := got.errModal.Context + "\n" + got.errModal.FriendlyError
	for _, want := range []string{"vol-a", "boom", "vol-b", "detachment not confirmed"} {
		if !strings.Contains(text, want) {
			t.Errorf("error modal missing %q: %s", want, text)
		}
	}
	found := false
	for _, msg := range commandMessages(cmd) {
		if _, ok := msg.(shared.RefreshServersMsg); ok {
			found = true
		}
	}
	if !found {
		t.Error("server list not refreshed")
	}
}
