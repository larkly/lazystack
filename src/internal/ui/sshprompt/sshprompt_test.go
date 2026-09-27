package sshprompt

import (
	tea "charm.land/bubbletea/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestIPPickerValidationAndConnect(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := New("web", []string{"203.0.113.1"}, []string{"2001:db8::1"}, []string{"10.0.0.1"}, " /tmp/test-key ", false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	if m.selectedIP() != "10.0.0.1" {
		t.Fatal("left must wrap")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	if m.selectedIP() != "203.0.113.1" {
		t.Fatal("right must wrap")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !m.ipPickerOpen || !strings.Contains(m.View(), "2001:db8::1") {
		t.Fatal("picker not rendered")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.ipPickerOpen || m.selectedIP() != "2001:db8::1" || m.focusField != fieldUser || !m.userInput.Focused() {
		t.Fatal("picker did not select and advance")
	}
	m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd != nil || !m.Active || !strings.Contains(m.View(), "Username cannot be empty") {
		t.Fatal("empty user accepted")
	}
	m.userInput.SetValue(" ubuntu ")
	m.focusField = fieldDebug
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.Active || cmd == nil {
		t.Fatal("connect not emitted")
	}
	got, ok := cmd().(SSHConnectMsg)
	want := SSHConnectMsg{User: "ubuntu", IP: "2001:db8::1", KeyPath: "/tmp/test-key", Debug: true, IgnoreHostKeys: true}
	if !ok || got != want {
		t.Fatalf("connect=%#v want=%#v", got, want)
	}
}

func TestKeyDiscoveryFilteringAndPicker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(filepath.Join(dir, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"z-key", "a-key", "a-key.pub", "known_hosts", "known_hosts.old", "config", "authorized_keys"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{filepath.Join(dir, "a-key"), filepath.Join(dir, "z-key")}
	if got := listSSHKeys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("keys=%v", got)
	}
	m := New("web", nil, nil, nil, "", false)
	m.SetSize(60, 40)
	m.focusField = fieldKeyPath
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !m.pickerOpen || m.keyInput.Focused() {
		t.Fatal("picker focus")
	}
	m.pickerFilter.SetValue("Z-KEY")
	if got := m.filteredPickerFiles(); len(got) != 1 || got[0] != want[1] {
		t.Fatal("case insensitive basename filter")
	}
	if !strings.Contains(m.View(), "z-key") {
		t.Fatal("key not rendered")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.pickerOpen || m.keyInput.Value() != want[1] || m.focusField != fieldDebug {
		t.Fatal("picker did not apply selected key")
	}
	m.focusField = fieldKeyPath
	m.openPicker()
	m.pickerFilter.SetValue("missing")
	if !strings.Contains(m.View(), "no keys found") {
		t.Fatal("empty picker")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if !m.Active || m.pickerOpen || !m.keyInput.Focused() {
		t.Fatal("escape should close picker only")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.Active {
		t.Fatal("escape should cancel modal")
	}
}

func TestKeyPickerFilterAcceptsJK(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "jack", "kube-a", "kube-b"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := New("web", nil, nil, nil, "", false)
	m.SetSize(80, 40)
	m.focusField = fieldKeyPath
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !m.pickerOpen {
		t.Fatal("picker not open")
	}
	for _, r := range "jack" {
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	if got := m.pickerFilter.Value(); got != "jack" {
		t.Fatalf("filter=%q", got)
	}
	m.pickerFilter.SetValue("")
	for _, r := range "kube" {
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	if got := m.pickerFilter.Value(); got != "kube" {
		t.Fatalf("filter=%q", got)
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.pickerCursor != 1 {
		t.Fatalf("down arrow cursor=%d", m.pickerCursor)
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.pickerOpen || m.keyInput.Value() != filepath.Join(dir, "kube-b") {
		t.Fatalf("enter selected %q", m.keyInput.Value())
	}
	m.focusField = fieldKeyPath
	m.openPicker()
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'k', Text: "k"}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if !m.Active || m.pickerOpen || m.keyInput.Value() != filepath.Join(dir, "kube-b") {
		t.Fatal("escape should close picker without changing the key")
	}
}
