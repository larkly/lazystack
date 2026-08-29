package shared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDebugEnabled_DefaultFalse(t *testing.T) {
	if DebugEnabled() {
		t.Error("DebugEnabled should return false by default")
	}
}

func TestEnableDebug_CreatesLogFile(t *testing.T) {
	// Capture original debug state
	origLogger := debugLogger
	origFile := debugFile
	defer func() {
		debugLogger = origLogger
		debugFile = origFile
	}()

	// Isolate the log path so parallel package test binaries cannot race
	// on the shared default location under the user cache dir.
	logPath := filepath.Join(t.TempDir(), "debug.log")
	t.Setenv("LAZYSTACK_DEBUG_LOG", logPath)

	err := EnableDebug()
	if err != nil {
		t.Fatalf("EnableDebug failed: %v", err)
	}
	defer func() {
		if debugFile != nil {
			debugFile.Close()
		}
	}()

	if !DebugEnabled() {
		t.Error("DebugEnabled should return true after EnableDebug")
	}
	if debugLogger == nil {
		t.Error("debugLogger should be non-nil after EnableDebug")
	}
}

func TestDebugf_WritesToLog(t *testing.T) {
	origLogger := debugLogger
	origFile := debugFile
	defer func() {
		debugLogger = origLogger
		debugFile = origFile
	}()

	logPath := filepath.Join(t.TempDir(), "debug.log")
	t.Setenv("LAZYSTACK_DEBUG_LOG", logPath)

	err := EnableDebug()
	if err != nil {
		t.Fatalf("EnableDebug failed: %v", err)
	}
	defer func() {
		if debugFile != nil {
			debugFile.Close()
		}
	}()

	Debugf("test message: %s", "hello")

	// Read the log file to verify the message was written
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read debug log: %v", err)
	}
	if !strings.Contains(string(data), "test message: hello") {
		t.Errorf("debug log missing expected message. Content: %s", string(data))
	}
}

func TestDebugf_SilentWhenNotEnabled(t *testing.T) {
	origLogger := debugLogger
	debugLogger = nil
	defer func() { debugLogger = origLogger }()

	// This should not panic and should not write anything
	Debugf("should be silent")
}
