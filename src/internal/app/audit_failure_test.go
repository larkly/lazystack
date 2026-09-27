package app

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larkly/lazystack/internal/audit"
)

// An enabled audit log that cannot be written must be surfaced once per
// session without hiding the result of the cloud action itself.
func TestAuditWriteFailureWarnsOnceAndKeepsResult(t *testing.T) {
	m, _ := actionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	// A regular file where the log directory should be: open and mkdir fail.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m.auditLogger = audit.NewLogger(filepath.Join(blocker, "audit.log"), true)

	res, cmd := m.Update(confirmed("pause", "srv"))
	m, _ = deliver(t, res.(Model), cmd)
	hint := m.statusBar.StickyHint
	if !strings.Contains(hint, "✓ Pause srv") {
		t.Errorf("action result hidden: %q", hint)
	}
	if !strings.Contains(strings.ToLower(hint), "audit") {
		t.Fatalf("audit failure not surfaced: %q", hint)
	}

	res, cmd = m.Update(confirmed("unpause", "srv"))
	m, _ = deliver(t, res.(Model), cmd)
	if hint := m.statusBar.StickyHint; !strings.Contains(hint, "✓ Unpause srv") || strings.Contains(strings.ToLower(hint), "audit") {
		t.Fatalf("second result should not repeat the audit warning: %q", hint)
	}
}
