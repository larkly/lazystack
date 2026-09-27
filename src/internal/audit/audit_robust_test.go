package audit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// collectIDs reads the active log and every retained backup and returns how
// often each ResourceID appears.
func collectIDs(t *testing.T, path string, maxFiles int) map[string]int {
	t.Helper()
	seen := map[string]int{}
	files := []string{path}
	for i := 1; i <= maxFiles; i++ {
		files = append(files, fmt.Sprintf("%s.%d", path, i))
	}
	for _, f := range files {
		entries, err := ReadEntries(f, 0)
		if err != nil {
			t.Fatalf("ReadEntries(%s): %v", f, err)
		}
		for _, e := range entries {
			seen[e.ResourceID]++
		}
	}
	return seen
}

func writeEntries(t *testing.T, l *Logger, writer string, n int) {
	for i := 0; i < n; i++ {
		e := Entry{Action: ActionCreate, ResourceType: "server", Result: "success",
			ResourceID: fmt.Sprintf("%s-%d", writer, i), ResourceName: strings.Repeat("x", 64)}
		if err := l.Log(e); err != nil {
			t.Errorf("writer %s: Log: %v", writer, err)
			return
		}
	}
}

func assertAllEntries(t *testing.T, seen map[string]int, writers, perWriter int) {
	t.Helper()
	missing, dup := 0, 0
	for w := 0; w < writers; w++ {
		for i := 0; i < perWriter; i++ {
			switch seen[fmt.Sprintf("w%d-%d", w, i)] {
			case 0:
				missing++
			case 1:
			default:
				dup++
			}
		}
	}
	if missing != 0 || dup != 0 {
		t.Fatalf("%d of %d entries missing, %d duplicated", missing, writers*perWriter, dup)
	}
}

// Independent Loggers on one path (as two lazystack instances would have)
// must not lose entries, even while rotating constantly.
func TestIndependentLoggersPreserveEntriesAcrossRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	const writers, perWriter, maxFiles = 8, 150, 300
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		l := NewLogger(path, true)
		l.maxSize = 2048
		l.maxFiles = maxFiles
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			writeEntries(t, l, fmt.Sprintf("w%d", w), perWriter)
		}(w)
	}
	wg.Wait()
	assertAllEntries(t, collectIDs(t, path, maxFiles), writers, perWriter)
}

const helperEnv = "LAZYSTACK_AUDIT_HELPER"

// TestAuditHelperProcess is the body of the child processes started by
// TestSeparateProcessesPreserveEntries; it does nothing in a normal run.
func TestAuditHelperProcess(t *testing.T) {
	spec := os.Getenv(helperEnv)
	if spec == "" {
		t.Skip("helper process only")
	}
	parts := strings.SplitN(spec, "|", 3)
	n, _ := strconv.Atoi(parts[1])
	l := NewLogger(parts[2], true)
	l.maxSize = 2048
	l.maxFiles = 300
	writeEntries(t, l, parts[0], n)
}

func TestSeparateProcessesPreserveEntries(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns processes")
	}
	path := filepath.Join(t.TempDir(), "audit.log")
	const writers, perWriter = 6, 150
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestAuditHelperProcess$", "-test.count=1")
		cmd.Env = append(os.Environ(), fmt.Sprintf("%s=w%d|%d|%s", helperEnv, w, perWriter, path))
		wg.Add(1)
		go func() {
			defer wg.Done()
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("helper: %v\n%s", err, out)
			}
		}()
	}
	wg.Wait()
	assertAllEntries(t, collectIDs(t, path, 300), writers, perWriter)
}

func TestLogWritesRecordAndNewlineTogether(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	l := NewLogger(path, true)
	var writes []int
	prev := writeRecord
	writeRecord = func(f *os.File, b []byte) (int, error) {
		writes = append(writes, len(b))
		return prev(f, b)
	}
	t.Cleanup(func() { writeRecord = prev })

	if err := l.Log(Entry{Action: ActionCreate, ResourceID: "one"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if len(writes) != 1 || writes[0] != len(data) || !strings.HasSuffix(string(data), "}\n") {
		t.Fatalf("writes=%v file=%q, want one write of the full JSON line", writes, data)
	}
}

// fillForRotation creates an active log already at the size limit.
func fillForRotation(t *testing.T, l *Logger) {
	t.Helper()
	if err := os.WriteFile(l.path, []byte(strings.Repeat("x", int(l.maxSize))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRotationFailureKeepsNewEntryAndReportsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	l := NewLogger(path, true)
	l.maxSize = 64
	fillForRotation(t, l)

	boom := errors.New("rename denied")
	prev := renameFile
	renameFile = func(oldPath, newPath string) error {
		if oldPath == path {
			return boom
		}
		return prev(oldPath, newPath)
	}
	t.Cleanup(func() { renameFile = prev })

	err := l.Log(Entry{Action: ActionDelete, ResourceID: "kept"})
	if !errors.Is(err, boom) {
		t.Fatalf("Log error = %v, want the rotation failure", err)
	}
	entries, _ := ReadEntries(path, 0)
	if len(entries) != 1 || entries[0].ResourceID != "kept" {
		t.Fatalf("entry lost after failed rotation: %+v", entries)
	}
}

func TestRotationReportsBackupFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remove func(string) error
		rename func(string, string) error
	}{
		{"remove oldest", func(string) error { return fs.ErrPermission }, nil},
		{"shift backup", nil, func(o, n string) error {
			if strings.HasSuffix(o, ".1") {
				return fs.ErrPermission
			}
			return os.Rename(o, n)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "audit.log")
			l := NewLogger(path, true)
			l.maxSize = 64
			l.maxFiles = 3
			for i := 1; i <= 3; i++ {
				if err := os.WriteFile(fmt.Sprintf("%s.%d", path, i), []byte("{}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			fillForRotation(t, l)
			prevRemove, prevRename := removeFile, renameFile
			if tc.remove != nil {
				removeFile = tc.remove
			}
			if tc.rename != nil {
				renameFile = tc.rename
			}
			t.Cleanup(func() { removeFile, renameFile = prevRemove, prevRename })

			err := l.Log(Entry{Action: ActionDelete, ResourceID: "new"})
			if !errors.Is(err, fs.ErrPermission) {
				t.Fatalf("Log error = %v, want the backup failure", err)
			}
			if seen := collectIDs(t, path, 3); seen["new"] != 1 {
				t.Errorf("new entry not appended after backup failure")
			}
		})
	}
}

func TestRotationWithMissingBackupsIsNormal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	l := NewLogger(path, true)
	l.maxSize = 64
	fillForRotation(t, l)
	if err := l.Log(Entry{Action: ActionCreate, ResourceID: "a"}); err != nil {
		t.Fatalf("rotation without backups: %v", err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("expected rotated .1: %v", err)
	}
}

func TestLogTightensExistingLogPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "share")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "audit.log")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	backup := path + ".1"
	if err := os.WriteFile(backup, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backup, 0o644); err != nil {
		t.Fatal(err)
	}

	l := NewLogger(path, true)
	l.maxSize = 64
	if err := l.Log(Entry{Action: ActionCreate, ResourceID: "a", ResourceName: strings.Repeat("x", 80)}); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("existing log mode = %o, want 600", info.Mode().Perm())
	}
	// Force a rotation: the old 0644 backup is shifted and must be private.
	if err := l.Log(Entry{Action: ActionCreate, ResourceID: "b"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, path + ".1", path + ".2"} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %o, want 600", filepath.Base(p), info.Mode().Perm())
		}
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o750 {
		t.Errorf("parent mode changed to %o", info.Mode().Perm())
	}
}

func TestLogNewFileAndParentArePrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "audit.log")
	l := NewLogger(path, true)
	if err := l.Log(Entry{Action: ActionCreate}); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("log mode = %o, want 600", info.Mode().Perm())
	}
	if info, _ := os.Stat(filepath.Dir(path)); info.Mode().Perm()&0o077 != 0 {
		t.Errorf("created parent mode = %o, want private", info.Mode().Perm())
	}
}

func TestLogChmodFailureIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	prev := chmodLog
	chmodLog = func(*os.File, os.FileMode) error { return fs.ErrPermission }
	t.Cleanup(func() { chmodLog = prev })
	if err := NewLogger(path, true).Log(Entry{Action: ActionCreate}); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Log error = %v, want permission failure", err)
	}
}
