package shared

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	debugLogger *log.Logger
	debugMu     sync.Mutex
	debugFile   *os.File
)

// EnableDebug opens a debug log file under the user cache directory.
func EnableDebug() error {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("determine user cache dir: %w", err)
	}
	dir := filepath.Join(cacheDir, "lazystack")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "debug.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	debugFile = f
	debugLogger = log.New(f, "", 0)
	debugLogger.Printf("=== debug started at %s ===", time.Now().Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "Debug log: %s\n", path)
	return nil
}

// DebugEnabled returns true if debug logging is active.
func DebugEnabled() bool {
	return debugLogger != nil
}

// Debugf writes a timestamped message to the debug log.
func Debugf(format string, args ...any) {
	if debugLogger == nil {
		return
	}
	debugMu.Lock()
	defer debugMu.Unlock()
	debugLogger.Printf("%s %s", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
}
