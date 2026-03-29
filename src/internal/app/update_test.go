package app

import (
	"testing"

	"github.com/larkly/lazystack/internal/config"
)

func newTestModel(version string, checkUpdate bool) Model {
	cfg := config.Defaults()
	return New(Options{
		Version:     version,
		CheckUpdate: checkUpdate,
		Config:      &cfg,
	})
}

func TestInit_CheckUpdateDisabledForDev(t *testing.T) {
	m := newTestModel("dev", true)
	cmd := m.Init()
	if cmd == nil {
		return // OK — no cmd when dev + no autoCloud
	}
	// If there is a cmd it should only be from autoCloud, not update check
	msg := cmd()
	if _, ok := msg.(UpdateAvailableMsg); ok {
		t.Error("update check should not run for dev builds")
	}
}

func TestInit_CheckUpdateDisabled(t *testing.T) {
	m := newTestModel("v0.0.1", false)
	cmd := m.Init()
	if cmd == nil {
		return
	}
	msg := cmd()
	if _, ok := msg.(UpdateAvailableMsg); ok {
		t.Error("update check should not run when CheckUpdate is false")
	}
}

func TestUpdateAvailableMsg_SetsHint(t *testing.T) {
	m := newTestModel("v0.0.1", true)
	m.width = 100
	m.height = 40

	msg := UpdateAvailableMsg{
		Latest:       "v0.1.1",
		DownloadURL:  "https://example.com/bin",
		ChecksumsURL: "https://example.com/SHA256SUMS",
	}

	result, _ := m.Update(msg)
	m = result.(Model)

	if m.activeModal != modalNone {
		t.Error("expected no modal for update available")
	}
	if m.latestVersion != "v0.1.1" {
		t.Errorf("latestVersion = %q, want %q", m.latestVersion, "v0.1.1")
	}
	if m.downloadURL != "https://example.com/bin" {
		t.Errorf("downloadURL = %q, want %q", m.downloadURL, "https://example.com/bin")
	}
	if m.statusBar.Hint == "" {
		t.Error("expected status bar hint about available upgrade")
	}
}

var errTest = &testError{}

type testError struct{}

func (e *testError) Error() string { return "test error" }
