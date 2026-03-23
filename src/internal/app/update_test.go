package app

import (
	"testing"
	"time"

	"github.com/larkly/lazystack/internal/ui/modal"
	"charm.land/bubbletea/v2"
)

func newTestModel(version string, checkUpdate bool) Model {
	return New(Options{
		Version:     version,
		CheckUpdate: checkUpdate,
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

func TestUpdateAvailableMsg_ShowsModal(t *testing.T) {
	m := newTestModel("v0.0.1", false)
	m.width = 100
	m.height = 40

	msg := UpdateAvailableMsg{
		Latest:       "v0.1.1",
		DownloadURL:  "https://example.com/bin",
		ChecksumsURL: "https://example.com/SHA256SUMS",
	}

	result, _ := m.Update(msg)
	m = result.(Model)

	if m.activeModal != modalConfirm {
		t.Error("expected confirm modal to be active")
	}
	if m.latestVersion != "v0.1.1" {
		t.Errorf("latestVersion = %q, want %q", m.latestVersion, "v0.1.1")
	}
	if m.downloadURL != "https://example.com/bin" {
		t.Errorf("downloadURL = %q, want %q", m.downloadURL, "https://example.com/bin")
	}
	if m.confirm.Action != "update" {
		t.Errorf("confirm.Action = %q, want %q", m.confirm.Action, "update")
	}
}

func TestConfirmAction_Update_SetsUpdating(t *testing.T) {
	m := newTestModel("v0.0.1", false)
	m.width = 100
	m.height = 40
	m.downloadURL = "https://example.com/bin"
	m.checksumsURL = "https://example.com/SHA256SUMS"
	m.latestVersion = "v0.1.1"
	m.activeModal = modalConfirm

	msg := modal.ConfirmAction{Action: "update", Confirm: true}
	result, cmd := m.Update(msg)
	m = result.(Model)

	if !m.updating {
		t.Error("expected updating to be true")
	}
	if cmd == nil {
		t.Error("expected a cmd to be returned for selfupdate.Apply")
	}
	if m.confirm.Title != "Updating" {
		t.Errorf("confirm.Title = %q, want %q", m.confirm.Title, "Updating")
	}
}

func TestConfirmAction_Update_Declined(t *testing.T) {
	m := newTestModel("v0.0.1", false)
	m.width = 100
	m.height = 40
	m.latestVersion = "v0.1.1"
	m.activeModal = modalConfirm

	msg := modal.ConfirmAction{Action: "update", Confirm: false}
	result, _ := m.Update(msg)
	m = result.(Model)

	if m.activeModal != modalNone {
		t.Error("expected modal to be dismissed")
	}
	if m.statusBar.Hint == "" {
		t.Error("expected status bar hint about available upgrade")
	}
}

func TestUpdateResultMsg_Success(t *testing.T) {
	m := newTestModel("v0.0.1", false)
	m.updating = true
	m.latestVersion = "v0.1.1"

	result, cmd := m.Update(UpdateResultMsg{Err: nil})
	m = result.(Model)

	if m.updating {
		t.Error("expected updating to be false")
	}
	if !m.restart {
		t.Error("expected restart to be true")
	}
	// cmd should be tea.Quit
	if cmd == nil {
		t.Error("expected quit cmd")
	}
}

func TestUpdateResultMsg_Failure(t *testing.T) {
	m := newTestModel("v0.0.1", false)
	m.width = 100
	m.height = 40
	m.updating = true

	result, _ := m.Update(UpdateResultMsg{Err: errTest})
	m = result.(Model)

	if m.updating {
		t.Error("expected updating to be false")
	}
	if m.activeModal != modalError {
		t.Error("expected error modal to be active")
	}
	if m.restart {
		t.Error("expected restart to be false on failure")
	}
}

func TestKeySwallowedWhileUpdating(t *testing.T) {
	m := newTestModel("v0.0.1", false)
	m.width = 100
	m.height = 40
	m.activeModal = modalConfirm
	m.updating = true
	m.refreshInterval = 5 * time.Second

	// Pressing 'y' should be swallowed
	keyMsg := tea.KeyPressMsg(tea.Key{Code: 'y', Text: "y"})
	result, cmd := m.Update(keyMsg)
	m = result.(Model)

	if cmd != nil {
		t.Error("expected nil cmd when keys are swallowed during update")
	}
	// Modal should still be active
	if m.activeModal != modalConfirm {
		t.Error("expected modal to remain active while updating")
	}
}

var errTest = &testError{}

type testError struct{}

func (e *testError) Error() string { return "test error" }
