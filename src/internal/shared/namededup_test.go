package shared

import (
	"testing"
)

func TestDeduplicateName_FirstClone(t *testing.T) {
	existing := map[string]bool{"my-server": true}
	got := DeduplicateName("my-server", existing)
	if got != "my-server-clone" {
		t.Errorf("expected 'my-server-clone', got %q", got)
	}
}

func TestDeduplicateName_EmptyExisting(t *testing.T) {
	existing := map[string]bool{}
	got := DeduplicateName("test", existing)
	if got != "test-clone" {
		t.Errorf("expected 'test-clone', got %q", got)
	}
}

func TestDeduplicateName_NilMap(t *testing.T) {
	got := DeduplicateName("test", nil)
	if got != "test-clone" {
		t.Errorf("expected 'test-clone' with nil map, got %q", got)
	}
}

func TestDeduplicateName_SecondCollision(t *testing.T) {
	existing := map[string]bool{
		"my-server":       true,
		"my-server-clone": true,
	}
	got := DeduplicateName("my-server", existing)
	if got != "my-server-clone-2" {
		t.Errorf("expected 'my-server-clone-2', got %q", got)
	}
}

func TestDeduplicateName_MultipleCollisions(t *testing.T) {
	existing := map[string]bool{
		"my-server":         true,
		"my-server-clone":   true,
		"my-server-clone-2": true,
	}
	got := DeduplicateName("my-server", existing)
	if got != "my-server-clone-3" {
		t.Errorf("expected 'my-server-clone-3', got %q", got)
	}
}

func TestDeduplicateName_BaseEndsWithClone(t *testing.T) {
	existing := map[string]bool{"already-clone": true}
	got := DeduplicateName("already-clone", existing)
	if got != "already-clone-clone" {
		t.Errorf("expected 'already-clone-clone', got %q", got)
	}
}
