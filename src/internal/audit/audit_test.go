package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogger_LogAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	l := NewLogger(path, true)
	l.maxSize = 1024 // 1 KB for test

	entry := Entry{
		Timestamp:    time.Now().UTC(),
		Cloud:        "test-cloud",
		Project:      "test-project",
		Action:       ActionCreate,
		ResourceType: "server",
		ResourceID:   "srv-123",
		ResourceName: "web-01",
		Result:       "success",
	}

	if err := l.Log(entry); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	entries, err := ReadEntries(path, 10)
	if err != nil {
		t.Fatalf("ReadEntries failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ResourceID != "srv-123" {
		t.Errorf("expected ResourceID srv-123, got %s", entries[0].ResourceID)
	}
}

func TestLogger_Disabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	l := NewLogger(path, false)
	if err := l.Log(Entry{Action: ActionDelete}); err != nil {
		t.Fatalf("Log on disabled logger should return nil, got %v", err)
	}
	entries, err := ReadEntries(path, 10)
	if err != nil {
		t.Fatalf("ReadEntries failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries when disabled, got %d", len(entries))
	}
}

func TestLogger_Rotate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	l := NewLogger(path, true)
	l.maxSize = 100 // very small to trigger rotation
	l.maxFiles = 2

	for i := 0; i < 5; i++ {
		entry := Entry{
			Timestamp:    time.Now().UTC(),
			Action:       ActionCreate,
			ResourceType: "server",
			ResourceName: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Result:       "success",
		}
		if err := l.Log(entry); err != nil {
			t.Fatalf("Log failed: %v", err)
		}
	}

	// .1 should exist after rotation
	if _, err := os.Stat(path + ".1"); os.IsNotExist(err) {
		t.Error("expected rotated file .1 to exist")
	}
}

func TestDefaultPath(t *testing.T) {
	p := DefaultPath()
	if p == "" {
		t.Fatal("DefaultPath returned empty")
	}
	if !filepath.IsAbs(p) {
		t.Errorf("DefaultPath should be absolute: %s", p)
	}
}
