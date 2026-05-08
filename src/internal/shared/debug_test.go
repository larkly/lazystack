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

	err := EnableDebug()
	if err != nil {
		t.Fatalf("EnableDebug failed: %v", err)
	}
	defer func() {
		if debugFile != nil {
			debugFile.Close()
		}
		cacheDir, _ := os.UserCacheDir()
		os.Remove(filepath.Join(cacheDir, "lazystack", "debug.log"))
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

	err := EnableDebug()
	if err != nil {
		t.Fatalf("EnableDebug failed: %v", err)
	}
	defer func() {
		if debugFile != nil {
			debugFile.Close()
		}
		cacheDir, _ := os.UserCacheDir()
		os.Remove(filepath.Join(cacheDir, "lazystack", "debug.log"))
	}()

	Debugf("test message: %s", "hello")

	// Read the log file to verify the message was written
	cacheDir, _ := os.UserCacheDir()
	logPath := filepath.Join(cacheDir, "lazystack", "debug.log")
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
