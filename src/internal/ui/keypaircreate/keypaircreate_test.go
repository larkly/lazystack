package keypaircreate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/compute"
)

const (
	testPriv = "-----BEGIN OPENSSH PRIVATE KEY-----\nNEW\n-----END OPENSSH PRIVATE KEY-----\n"
	testPub  = "ssh-ed25519 AAAANEW generated"
)

// generatedModel returns a model showing a freshly generated key pair, with
// HOME pointed at a temp dir. It returns the model and the home dir.
func generatedModel(t *testing.T, name string) (Model, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("permission bits and HOME handling differ on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	m := New(nil)
	m.SetSize(100, 40)
	m, _ = m.Update(keypairCreatedMsg{kp: &compute.KeyPairFull{Name: name, PrivateKey: testPriv, PublicKey: testPub}})
	if !m.showPrivateKey {
		t.Fatal("private key view not shown")
	}
	return m, home
}

func openSave(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if !m.showSaveInput {
		t.Fatal("save input not shown")
	}
	return m
}

func enter(m Model) Model {
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return m
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestSaveCreatesSSHDirAndKeyFiles(t *testing.T) {
	m, home := generatedModel(t, "lazy-test")
	m = enter(openSave(t, m))

	sshDir := filepath.Join(home, ".ssh")
	path := filepath.Join(sshDir, "lazy-test")
	if m.saveErr != "" || m.savedPath != path || !m.publicKeySaved {
		t.Fatalf("saveErr=%q savedPath=%q publicKeySaved=%v", m.saveErr, m.savedPath, m.publicKeySaved)
	}
	if got := mode(t, sshDir); got != 0o700 {
		t.Fatalf("~/.ssh mode = %o, want 700", got)
	}
	if got := mode(t, path); got != 0o600 {
		t.Fatalf("private key mode = %o, want 600", got)
	}
	if got := mode(t, path+".pub"); got != 0o644 {
		t.Fatalf("public key mode = %o, want 644", got)
	}
	if b, _ := os.ReadFile(path); string(b) != testPriv {
		t.Fatalf("private key content = %q", b)
	}
	if b, _ := os.ReadFile(path + ".pub"); string(b) != testPub {
		t.Fatalf("public key content = %q", b)
	}
}

func TestSaveNeverOverwritesExistingPrivateKey(t *testing.T) {
	m, home := generatedModel(t, "id_ed25519")
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sshDir, "id_ed25519")
	if err := os.WriteFile(path, []byte("REAL KEY"), 0o644); err != nil {
		t.Fatal(err)
	}

	m = enter(openSave(t, m))

	if b, _ := os.ReadFile(path); string(b) != "REAL KEY" {
		t.Fatalf("existing private key was overwritten: %q", b)
	}
	if _, err := os.Stat(path + ".pub"); !os.IsNotExist(err) {
		t.Fatalf("public key written although the private key path exists: %v", err)
	}
	if m.savedPath != "" || !strings.Contains(m.saveErr, "already exists") {
		t.Fatalf("savedPath=%q saveErr=%q, want an 'already exists' error", m.savedPath, m.saveErr)
	}
	if !m.showSaveInput {
		t.Fatal("save input closed; the user should be able to enter another path")
	}
	if !strings.Contains(m.View(), "already exists") {
		t.Fatal("error not rendered")
	}

	// A different path then succeeds.
	m.savePathInput.SetValue(filepath.Join(sshDir, "id_other"))
	m = enter(m)
	if m.saveErr != "" || m.savedPath != filepath.Join(sshDir, "id_other") {
		t.Fatalf("retry with new path: saveErr=%q savedPath=%q", m.saveErr, m.savedPath)
	}
}

func TestSaveNeverOverwritesExistingPublicKey(t *testing.T) {
	m, home := generatedModel(t, "id_rsa")
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sshDir, "id_rsa")
	if err := os.WriteFile(path+".pub", []byte("REAL PUB"), 0o644); err != nil {
		t.Fatal(err)
	}

	m = enter(openSave(t, m))

	if b, _ := os.ReadFile(path + ".pub"); string(b) != "REAL PUB" {
		t.Fatalf("existing public key was overwritten: %q", b)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("private key written although the public key path exists: %v", err)
	}
	if m.savedPath != "" || !strings.Contains(m.saveErr, "already exists") {
		t.Fatalf("savedPath=%q saveErr=%q", m.savedPath, m.saveErr)
	}
}

func TestSaveExpandsTilde(t *testing.T) {
	m, home := generatedModel(t, "k")
	m = openSave(t, m)
	m.savePathInput.SetValue("~/keys/k")
	m = enter(m)
	want := filepath.Join(home, "keys", "k")
	if m.saveErr != "" || m.savedPath != want {
		t.Fatalf("saveErr=%q savedPath=%q, want %q", m.saveErr, m.savedPath, want)
	}
	if got := mode(t, want); got != 0o600 {
		t.Fatalf("private key mode = %o, want 600", got)
	}
}
