package ssh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildArgsIncludesIgnoreHostKeyFlags(t *testing.T) {
	args := BuildArgs(Options{
		User:           "ubuntu",
		IP:             "192.0.2.10",
		KeyPath:        "/tmp/id_rsa",
		IgnoreHostKeys: true,
	})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "StrictHostKeyChecking=no") {
		t.Fatalf("args missing StrictHostKeyChecking=no: %v", args)
	}
	if !strings.Contains(joined, "UserKnownHostsFile=/dev/null") {
		t.Fatalf("args missing UserKnownHostsFile=/dev/null: %v", args)
	}
}

func TestBuildCommandStringIncludesIgnoreHostKeyFlags(t *testing.T) {
	cmd := BuildCommandString(Options{
		User:           "ubuntu",
		IP:             "192.0.2.10",
		IgnoreHostKeys: true,
	})

	if !strings.Contains(cmd, "StrictHostKeyChecking=no") {
		t.Fatalf("command missing StrictHostKeyChecking=no: %s", cmd)
	}
	if !strings.Contains(cmd, "UserKnownHostsFile=/dev/null") {
		t.Fatalf("command missing UserKnownHostsFile=/dev/null: %s", cmd)
	}
}

func TestBuildCommandString_QuotesShellInjection(t *testing.T) {
	cmd := BuildCommandString(Options{
		User:    "foo;rm -rf ~",
		IP:      "192.0.2.10",
		KeyPath: "/home/u/.ssh/id_rsa",
	})

	want := `'foo;rm -rf ~@192.0.2.10'`
	if !strings.Contains(cmd, want) {
		t.Fatalf("command does not safely quote user@ip: want substring %q in %q", want, cmd)
	}
	// The unquoted, executable form must never appear.
	if strings.Contains(cmd, " foo;rm") {
		t.Fatalf("injection payload appears unquoted: %q", cmd)
	}
}

func TestBuildCommandString_QuotesKeyPathWithSpaces(t *testing.T) {
	cmd := BuildCommandString(Options{
		User:    "ubuntu",
		IP:      "192.0.2.10",
		KeyPath: "/tmp/my keys/id_rsa test",
	})

	want := `-i '/tmp/my keys/id_rsa test'`
	if !strings.Contains(cmd, want) {
		t.Fatalf("key path with spaces not quoted: want substring %q in %q", want, cmd)
	}
}

func TestBuildCommandString_EscapesEmbeddedQuotes(t *testing.T) {
	cmd := BuildCommandString(Options{
		User:    "ubuntu",
		IP:      "192.0.2.10",
		KeyPath: "/tmp/it's/id",
	})

	want := `-i '/tmp/it'\''s/id'`
	if !strings.Contains(cmd, want) {
		t.Fatalf("embedded quote not escaped: want substring %q in %q", want, cmd)
	}
}

func TestBuildCommandString_NormalPathStaysClean(t *testing.T) {
	cmd := BuildCommandString(Options{
		User:    "ubuntu",
		IP:      "192.0.2.10",
		KeyPath: "/home/u/.ssh/id_rsa",
	})

	if strings.Contains(cmd, "'") {
		t.Fatalf("normal components should not be quoted: %q", cmd)
	}
	if !strings.Contains(cmd, "ubuntu@192.0.2.10") {
		t.Fatalf("command missing plain user@ip: %q", cmd)
	}
	if !strings.Contains(cmd, "-i /home/u/.ssh/id_rsa") {
		t.Fatalf("command missing plain key path: %q", cmd)
	}
}

func TestBuildArgs_Composition(t *testing.T) {
	args := BuildArgs(Options{
		User:           "ubuntu",
		IP:             "192.0.2.10",
		KeyPath:        "/tmp/id_rsa",
		Debug:          true,
		IgnoreHostKeys: true,
	})

	if len(args) == 0 || args[0] != "-t" {
		t.Fatalf("args should start with base args (-t), got %v", args)
	}
	if !strings.Contains(strings.Join(args, " "), "ConnectTimeout=10") {
		t.Fatalf("args missing base ConnectTimeout: %v", args)
	}
	if !containsPair(args, "-v") {
		t.Fatalf("args missing -v for Debug: %v", args)
	}
	if !containsPair(args, "-i") || !containsValue(args, "/tmp/id_rsa") {
		t.Fatalf("args missing key path -i /tmp/id_rsa: %v", args)
	}
	if last := args[len(args)-1]; last != "ubuntu@192.0.2.10" {
		t.Fatalf("last arg should be user@ip, got %q", last)
	}
}

func containsPair(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func containsValue(args []string, val string) bool {
	for _, a := range args {
		if a == val {
			return true
		}
	}
	return false
}

func TestChooseIP_Priority(t *testing.T) {
	cases := []struct {
		name     string
		floating []string
		ipv6     []string
		ipv4     []string
		want     string
	}{
		{"floating wins", []string{"172.16.0.1"}, []string{"fd00::1"}, []string{"10.0.0.1"}, "172.16.0.1"},
		{"ipv6 over ipv4", nil, []string{"fd00::1"}, []string{"10.0.0.1"}, "fd00::1"},
		{"ipv4 fallback", nil, nil, []string{"10.0.0.1"}, "10.0.0.1"},
		{"first of each list", []string{"172.16.0.1", "172.16.0.2"}, []string{"fd00::1", "fd00::2"}, []string{"10.0.0.1", "10.0.0.2"}, "172.16.0.1"},
		{"all empty", nil, nil, nil, ""},
		{"all nil", []string{}, []string{}, []string{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ChooseIP(tc.floating, tc.ipv6, tc.ipv4); got != tc.want {
				t.Errorf("ChooseIP(%v, %v, %v) = %q, want %q", tc.floating, tc.ipv6, tc.ipv4, got, tc.want)
			}
		})
	}
}

func TestFindKeyPath_ResolvesKeyInSshDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(sshDir, "mykey")
	if err := os.WriteFile(keyPath, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := FindKeyPath("mykey"); got != keyPath {
		t.Errorf("FindKeyPath(mykey) = %q, want %q", got, keyPath)
	}
	if got := FindKeyPath("missing"); got != "" {
		t.Errorf("FindKeyPath(missing) = %q, want empty", got)
	}
}

func TestFindKeyPath_RejectsTraversalNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Simulate an attacker-controlled file outside ~/.ssh that the name
	// would otherwise reach via traversal.
	outside := filepath.Join(home, "hosts")
	if err := os.WriteFile(outside, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"../../hosts",
		"../../../etc/hosts",
		"foo/bar",
		"/etc/hosts",
		"..",
		".",
		`foo\..\hosts`,
	} {
		if got := FindKeyPath(name); got != "" {
			t.Errorf("FindKeyPath(%q) = %q, want empty (traversal must be rejected)", name, got)
		}
	}
}
