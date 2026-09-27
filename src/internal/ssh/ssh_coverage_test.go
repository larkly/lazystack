package ssh

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestChooseIPPreservesFirstCandidate(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		floating, ipv6, ipv4 []string
		want                 string
	}{
		{"first IPv6", []string{}, []string{"2001:db8::2", "2001:db8::1"}, []string{"192.0.2.1"}, "2001:db8::2"},
		{"first IPv4", nil, []string{}, []string{"192.0.2.2", "192.0.2.1"}, "192.0.2.2"},
		{"empty first floating skipped", []string{"", "192.0.2.2"}, []string{"2001:db8::1"}, nil, "192.0.2.2"},
		{"empty IPv6 skipped", nil, []string{"", "2001:db8::1"}, []string{"192.0.2.1"}, "2001:db8::1"},
		{"all-empty floating falls to IPv4", []string{"", "  "}, nil, []string{"192.0.2.1"}, "192.0.2.1"},
		{"all empty yields no target", []string{""}, []string{" "}, []string{""}, ""},
		{"floating still beats IPv6", []string{"198.51.100.7"}, []string{"2001:db8::1"}, []string{"192.0.2.1"}, "198.51.100.7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ChooseIP(tc.floating, tc.ipv6, tc.ipv4); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestFindKeyPathCandidateResolution(t *testing.T) {
	for _, tc := range []struct {
		name        string
		files, dirs []string
		want        string
	}{
		{"exact wins", []string{"key", "key.pem", "id_key"}, nil, "key"},
		{"pem wins over prefixed", []string{"key.pem", "id_key"}, nil, "key.pem"},
		{"prefix fallback", []string{"id_key"}, nil, "id_key"},
		{"skip exact directory", []string{"key.pem"}, []string{"key"}, "key.pem"},
		{"skip both directories", []string{"id_key"}, []string{"key", "key.pem"}, "id_key"},
		{"directories are not keys", nil, []string{"key", "key.pem", "id_key"}, ""},
		{"missing directory", nil, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := filepath.Join(home, ".ssh")
			if len(tc.files)+len(tc.dirs) > 0 {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("test key"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range tc.dirs {
				if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
					t.Fatal(err)
				}
			}
			want := ""
			if tc.want != "" {
				want = filepath.Join(dir, tc.want)
			}
			if got := FindKeyPath("key"); got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		})
	}
}

func TestFindKeyPathUnsafeNamesCannotResolveExistingFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(home, "outside"), filepath.Join(dir, "nested", "key"), filepath.Join(dir, `windows\key`)} {
		if err := os.WriteFile(path, []byte("test key"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"", "../outside", filepath.Join(home, "outside"), "nested/key", "nested/../nested/key", `windows\key`, ".", ".."} {
		if got := FindKeyPath(name); got != "" {
			t.Errorf("unsafe %q resolved to %q", name, got)
		}
	}
}

func TestFindKeyPathMissingHome(t *testing.T) {
	t.Setenv("HOME", "")
	if got := FindKeyPath("key"); got != "" {
		t.Fatalf("got %q without HOME", got)
	}
}

func TestSSHArgumentCompositionExact(t *testing.T) {
	base := []string{"-t", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3"}
	for _, debug := range []bool{false, true} {
		for _, ignore := range []bool{false, true} {
			for _, key := range []string{"", "/tmp/my key's.pem"} {
				opts := Options{User: "ubuntu", IP: "2001:db8::1", KeyPath: key, Debug: debug, IgnoreHostKeys: ignore}
				want := append([]string{}, base...)
				if debug {
					want = append(want, "-v")
				}
				if ignore {
					want = append(want, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null")
				}
				if key != "" {
					want = append(want, "-i", key)
				}
				want = append(want, "ubuntu@2001:db8::1")
				if got := BuildArgs(opts); !reflect.DeepEqual(got, want) {
					t.Errorf("opts=%+v: got %#v want %#v", opts, got, want)
				}
				// Clipboard commands deliberately omit the interactive debug flag today.
				cmd := "ssh -t -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=3"
				if ignore {
					cmd += " -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
				}
				if key != "" {
					cmd += ` -i '/tmp/my key'\''s.pem'`
				}
				cmd += " ubuntu@2001:db8::1"
				if got := BuildCommandString(opts); got != cmd {
					t.Errorf("opts=%+v: got %q want %q", opts, got, cmd)
				}
			}
		}
	}
	// Each call owns its argument slice; callers may safely modify it.
	first := BuildArgs(Options{})
	first[0] = "changed"
	if got := BuildArgs(Options{}); got[0] != "-t" || got[len(got)-1] != "@" {
		t.Fatalf("default arguments = %#v", got)
	}
}
