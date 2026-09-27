package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/larkly/lazystack/internal/config"
)

func TestReportConfigLoadPrintsWarnings(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.Defaults()
	cfg.Warnings = []string{"keybinding attach=\"ctrl+a\" uses a reserved key"}
	reportConfigLoad(&buf, cfg, errors.New("boom"))
	out := buf.String()
	if !strings.Contains(out, "failed to load config: boom") {
		t.Errorf("load error not reported: %q", out)
	}
	if !strings.Contains(out, "reserved key") {
		t.Errorf("config warning not reported: %q", out)
	}
}

// A failed exec must be reported and turned into a nonzero exit status. The
// target files make the real execve fail with ENOENT, EACCES and ENOEXEC.
func TestRestartFailureIsReported(t *testing.T) {
	dir := t.TempDir()
	notExec := filepath.Join(dir, "not-executable")
	if err := os.WriteFile(notExec, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	garbage := filepath.Join(dir, "garbage")
	if err := os.WriteFile(garbage, []byte{0, 1, 2, 3, 4, 5, 6, 7}, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path string
		want       error
	}{
		{"ENOENT", filepath.Join(dir, "missing"), syscall.ENOENT},
		{"EACCES", notExec, syscall.EACCES},
		{"ENOEXEC", garbage, syscall.ENOEXEC},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prev := executable
			executable = func() (string, error) { return tc.path, nil }
			t.Cleanup(func() { executable = prev })

			var buf bytes.Buffer
			if code := restartOrReport(&buf); code == 0 {
				t.Fatal("failed restart returned exit status 0")
			}
			if !strings.Contains(buf.String(), "restart failed") || !strings.Contains(buf.String(), tc.want.Error()) {
				t.Errorf("stderr = %q, want restart failure mentioning %v", buf.String(), tc.want)
			}
		})
	}
}

func TestRestartExecutableLookupFailureIsReported(t *testing.T) {
	prev := executable
	executable = func() (string, error) { return "", errors.New("no /proc") }
	t.Cleanup(func() { executable = prev })
	var buf bytes.Buffer
	if code := restartOrReport(&buf); code == 0 || !strings.Contains(buf.String(), "no /proc") {
		t.Fatalf("code=%d stderr=%q", code, buf.String())
	}
}

const restartStageEnv = "LAZYSTACK_RESTART_STAGE"

// TestRestartHelperProcess runs inside a child process: stage 1 restarts
// itself, stage 2 proves it is the replacement image.
func TestRestartHelperProcess(t *testing.T) {
	switch os.Getenv(restartStageEnv) {
	case "1":
		os.Setenv(restartStageEnv, "2")
		fmt.Printf("stage1 pid=%d\n", os.Getpid())
		os.Exit(restartOrReport(os.Stderr) + 10)
	case "2":
		fmt.Printf("replaced pid=%d\n", os.Getpid())
		os.Exit(0)
	default:
		t.Skip("helper process only")
	}
}

func TestRestartSuccessReplacesProcess(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestRestartHelperProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), restartStageEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}
	var stage1, replaced int
	for _, line := range strings.Split(string(out), "\n") {
		fmt.Sscanf(line, "stage1 pid=%d", &stage1)
		fmt.Sscanf(line, "replaced pid=%d", &replaced)
	}
	if replaced == 0 || replaced != stage1 {
		t.Fatalf("exec did not replace the process in place:\n%s", out)
	}
}

func TestReportConfigLoadSilentWhenClean(t *testing.T) {
	var buf bytes.Buffer
	reportConfigLoad(&buf, config.Defaults(), nil)
	if buf.Len() != 0 {
		t.Errorf("unexpected output: %q", buf.String())
	}
}
