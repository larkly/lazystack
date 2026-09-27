package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larkly/lazystack/internal/config"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/modal"
)

// autoCloudModel returns a model started with --cloud alpha, with
// clouds.yaml listing alpha and beta.
func autoCloudModel(t *testing.T) Model {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clouds.yaml")
	if err := os.WriteFile(path, []byte("clouds:\n  alpha:\n    region_name: r1\n  beta:\n    region_name: r1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OS_CLIENT_CONFIG_FILE", path)

	cfg := config.Defaults()
	m := New(Options{Version: "test", Cloud: "alpha", Config: &cfg})
	m.width, m.height = 120, 40
	if !strings.Contains(m.viewContent(), "Connecting to alpha") {
		t.Fatalf("startup placeholder not shown:\n%s", m.viewContent())
	}
	return m
}

// assertCloudListShown checks that the cloud picker renders its list rather
// than the automatic connection placeholder.
func assertCloudListShown(t *testing.T, m Model) {
	t.Helper()
	if m.view != viewCloudPicker {
		t.Fatalf("view=%v want cloud picker", m.view)
	}
	content := m.viewContent()
	if strings.Contains(content, "Connecting to") || !strings.Contains(content, "beta") {
		t.Fatalf("cloud picker shows the connecting placeholder instead of the list:\n%s", content)
	}
}

// After the automatic startup connection, C must show the cloud list.
func TestCloudPickerAfterAutoConnectShowsList(t *testing.T) {
	m := autoCloudModel(t)
	f := newCloudFixture(t, `[]`, `[]`, 200)
	res, _ := m.Update(f.connected())
	m = res.(Model)
	if m.view != viewServerList {
		t.Fatalf("view=%v want server list after connect", m.view)
	}

	m, cmd := updateWithin(t, m, press("C"))
	if cmd == nil {
		t.Fatal("C did not list clouds")
	}
	m, _ = updateWithin(t, m, cmd())
	assertCloudListShown(t, m)
}

// A failed automatic connection falls back to the cloud list once the
// error is dismissed, instead of "Connecting to" forever.
func TestFailedAutoConnectShowsCloudList(t *testing.T) {
	m := autoCloudModel(t)
	res, _ := m.Update(shared.CloudConnectErrMsg{Err: errors.New("auth failed")})
	m = res.(Model)
	res, _ = m.Update(modal.ErrorDismissedMsg{})
	m = res.(Model)
	assertCloudListShown(t, m)
}
