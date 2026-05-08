package compute

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// instanceActionsFixture is a minimal nova API paginated instance action list response.
const instanceActionsFixture = `{
  "instanceActions": [
    {
      "action": "create",
      "request_id": "req-abc123",
      "user_id": "user-001",
      "start_time": "2026-01-15T10:00:00",
      "message": "Instance created"
    },
    {
      "action": "reboot",
      "request_id": "req-def456",
      "user_id": "user-002",
      "start_time": "2026-01-15T11:00:00",
      "message": null
    },
    {
      "action": "resize",
      "request_id": "req-ghi789",
      "user_id": "user-001",
      "start_time": "2026-01-15T12:30:00",
      "message": "Resizing from m1.small to m1.large"
    }
  ]
}`

func TestListActions(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/servers/") && strings.Contains(r.URL.Path, "/os-instance-actions") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(instanceActionsFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeNovaClient(handler)
	ctx := context.Background()

	actions, err := ListActions(ctx, client, "server-001")
	if err != nil {
		t.Fatalf("ListActions() error: %v", err)
	}
	if len(actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(actions))
	}

	// Verify first action: create
	a1 := actions[0]
	if a1.Action != "create" {
		t.Errorf("unexpected action: %s", a1.Action)
	}
	if a1.RequestID != "req-abc123" {
		t.Errorf("unexpected request_id: %s", a1.RequestID)
	}
	if a1.UserID != "user-001" {
		t.Errorf("unexpected user_id: %s", a1.UserID)
	}

	// Verify second action: reboot (nil message)
	a2 := actions[1]
	if a2.Action != "reboot" {
		t.Errorf("unexpected second action: %s", a2.Action)
	}
	if a2.Message != "" {
		t.Errorf("expected empty message for reboot, got %q", a2.Message)
	}

	// Verify third action: resize
	a3 := actions[2]
	if a3.Action != "resize" {
		t.Errorf("unexpected third action: %s", a3.Action)
	}
	if a3.Message == "" {
		t.Error("expected non-empty message for resize")
	}
}

func TestListActions_EmptyServer(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"instanceActions": []}`))
	})

	client := fakeNovaClient(handler)
	ctx := context.Background()

	actions, err := ListActions(ctx, client, "server-empty")
	if err != nil {
		t.Fatalf("ListActions() error: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("expected 0 actions for empty server, got %d", len(actions))
	}
}
