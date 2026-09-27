package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

// cloudsEnv isolates clouds.yaml discovery: HOME, XDG_CONFIG_HOME, the
// system path and the working directory all point into fresh temp dirs.
type cloudsEnv struct {
	home, cwd string
}

func newCloudsEnv(t *testing.T) cloudsEnv {
	t.Helper()
	e := cloudsEnv{home: t.TempDir(), cwd: t.TempDir()}
	t.Setenv("HOME", e.home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OS_CLIENT_CONFIG_FILE", "")
	t.Setenv("OS_CLOUD", "")
	t.Setenv("OS_REGION_NAME", "")
	prev := systemCloudsYaml
	systemCloudsYaml = filepath.Join(t.TempDir(), "etc", "openstack", "clouds.yaml")
	t.Cleanup(func() { systemCloudsYaml = prev })
	t.Chdir(e.cwd)
	return e
}

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func cloudYAML(name, authURL string) string {
	return "clouds:\n  " + name + ":\n    auth:\n      auth_url: " + authURL +
		"\n      username: u\n      password: from-clouds-yaml\n      project_name: p\n      user_domain_name: Default\n"
}

// authRecorder is a keystone stub that records every token request and its
// password, then rejects it so Connect stops right after authentication.
type authRecorder struct {
	srv      *httptest.Server
	hits     atomic.Int32
	password atomic.Value
}

func newAuthRecorder(t *testing.T) *authRecorder {
	t.Helper()
	a := &authRecorder{}
	a.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.hits.Add(1)
		var body struct {
			Auth struct {
				Identity struct {
					Password struct {
						User struct {
							Password string `json:"password"`
						} `json:"user"`
					} `json:"password"`
				} `json:"identity"`
			} `json:"auth"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		a.password.Store(body.Auth.Identity.Password.User.Password)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(a.srv.Close)
	return a
}

func (a *authRecorder) url() string { return a.srv.URL + "/v3" }

func TestCloudsYamlPaths_XDGConfigHome(t *testing.T) {
	newCloudsEnv(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	paths := CloudsYamlPaths()
	if paths[0] != filepath.Join(xdg, "openstack", "clouds.yaml") {
		t.Errorf("first path = %s, want XDG config", paths[0])
	}
	for _, p := range paths {
		if strings.HasPrefix(p, os.Getenv("HOME")) {
			t.Errorf("HOME path %s searched although XDG_CONFIG_HOME is set", p)
		}
	}

	// A relative XDG_CONFIG_HOME is invalid per the spec and is ignored.
	t.Setenv("XDG_CONFIG_HOME", "relative/dir")
	paths = CloudsYamlPaths()
	if want := filepath.Join(os.Getenv("HOME"), ".config", "openstack", "clouds.yaml"); paths[0] != want {
		t.Errorf("first path = %s, want HOME fallback %s", paths[0], want)
	}
}

func TestListCloudNames_XDGPreferredOverHome(t *testing.T) {
	e := newCloudsEnv(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(xdg, "openstack", "clouds.yaml"), cloudYAML("xdgcloud", "https://xdg.example.com/v3"))
	writeFile(t, filepath.Join(e.home, ".config", "openstack", "clouds.yaml"), cloudYAML("homecloud", "https://home.example.com/v3"))

	names, err := ListCloudNames()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"xdgcloud"}) {
		t.Errorf("names = %v, want [xdgcloud]", names)
	}
}

func TestListCloudNames_UnsetXDGUsesHome(t *testing.T) {
	e := newCloudsEnv(t)
	writeFile(t, filepath.Join(e.home, ".config", "openstack", "clouds.yaml"), cloudYAML("homecloud", "https://home.example.com/v3"))
	writeFile(t, filepath.Join(e.cwd, "clouds.yaml"), cloudYAML("cwdcloud", "https://cwd.example.com/v3"))

	names, err := ListCloudNames()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"homecloud"}) {
		t.Errorf("names = %v, want [homecloud] (HOME before CWD)", names)
	}
}

// An empty first file followed by a valid later file must be treated the
// same way by listing and by connecting, and secure.yaml must come from the
// directory of the clouds.yaml that was actually selected.
func TestEmptyFirstCloudsYamlListAndConnectConsistently(t *testing.T) {
	e := newCloudsEnv(t)
	auth := newAuthRecorder(t)
	homeDir := filepath.Join(e.home, ".config", "openstack")
	writeFile(t, filepath.Join(homeDir, "clouds.yaml"), "clouds: {}\n")
	writeFile(t, filepath.Join(homeDir, "secure.yaml"), "clouds:\n  cwdcloud:\n    auth:\n      password: wrong-directory\n")
	writeFile(t, filepath.Join(e.cwd, "clouds.yaml"), cloudYAML("cwdcloud", auth.url()))
	writeFile(t, filepath.Join(e.cwd, "secure.yaml"), "clouds:\n  cwdcloud:\n    auth:\n      password: paired-secret\n")

	names, err := ListCloudNames()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"cwdcloud"}) {
		t.Fatalf("names = %v, want [cwdcloud]", names)
	}

	if _, err := Connect(context.Background(), "cwdcloud"); err == nil {
		t.Fatal("expected the stub's 401 to fail Connect")
	}
	if auth.hits.Load() == 0 {
		t.Fatal("Connect never authenticated against the listed cloud")
	}
	if got, _ := auth.password.Load().(string); got != "paired-secret" {
		t.Errorf("password = %q, want the secure.yaml next to the selected clouds.yaml", got)
	}
}

func TestMalformedCloudsYamlIsNotBypassed(t *testing.T) {
	e := newCloudsEnv(t)
	auth := newAuthRecorder(t)
	writeFile(t, filepath.Join(e.home, ".config", "openstack", "clouds.yaml"), "clouds:\n  broken: [\n")
	writeFile(t, filepath.Join(e.cwd, "clouds.yaml"), cloudYAML("cwdcloud", auth.url()))

	if _, err := ListCloudNames(); err == nil {
		t.Error("ListCloudNames bypassed a malformed clouds.yaml")
	}
	if _, err := Connect(context.Background(), "cwdcloud"); err == nil {
		t.Error("Connect bypassed a malformed clouds.yaml")
	}
	if auth.hits.Load() != 0 {
		t.Error("Connect authenticated using a later clouds.yaml")
	}
}

func TestOverrideFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, path string)
	}{
		{"missing", func(t *testing.T, path string) {}},
		{"empty", func(t *testing.T, path string) { writeFile(t, path, "") }},
		{"no clouds", func(t *testing.T, path string) { writeFile(t, path, "clouds: {}\n") }},
		{"malformed", func(t *testing.T, path string) { writeFile(t, path, "clouds:\n  x: [\n") }},
		{"unreadable", func(t *testing.T, path string) {
			if os.Geteuid() == 0 {
				t.Skip("root can read mode 000 files")
			}
			writeFile(t, path, cloudYAML("override", "https://override.example.com/v3"))
			if err := os.Chmod(path, 0); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newCloudsEnv(t)
			auth := newAuthRecorder(t)
			override := filepath.Join(t.TempDir(), "override.yaml")
			tc.setup(t, override)
			t.Setenv("OS_CLIENT_CONFIG_FILE", override)
			// Valid configs everywhere the old fallback would have looked.
			writeFile(t, filepath.Join(e.cwd, "clouds.yaml"), cloudYAML("planted", auth.url()))
			writeFile(t, filepath.Join(e.home, ".config", "openstack", "clouds.yaml"), cloudYAML("planted", auth.url()))
			writeFile(t, systemCloudsYaml, cloudYAML("planted", auth.url()))

			names, err := ListCloudNames()
			if err == nil {
				t.Errorf("ListCloudNames fell back past the override: %v", names)
			} else if !strings.Contains(err.Error(), "OS_CLIENT_CONFIG_FILE") {
				t.Errorf("error %q does not name OS_CLIENT_CONFIG_FILE", err)
			}
			if _, err := Connect(context.Background(), "planted"); err == nil {
				t.Error("Connect succeeded with an unusable override")
			}
			if auth.hits.Load() != 0 {
				t.Error("Connect authenticated with a fallback clouds.yaml")
			}
		})
	}
}

func TestValidOverrideIsExclusive(t *testing.T) {
	e := newCloudsEnv(t)
	auth := newAuthRecorder(t)
	override := writeFile(t, filepath.Join(t.TempDir(), "override.yaml"), cloudYAML("mine", "https://mine.example.com/v3"))
	t.Setenv("OS_CLIENT_CONFIG_FILE", override)
	writeFile(t, filepath.Join(e.cwd, "clouds.yaml"), cloudYAML("planted", auth.url()))

	names, err := ListCloudNames()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"mine"}) {
		t.Errorf("names = %v, want [mine]", names)
	}
	if paths := CloudsYamlPaths(); !reflect.DeepEqual(paths, []string{override}) {
		t.Errorf("paths = %v, want only the override", paths)
	}
	if _, err := Connect(context.Background(), "planted"); err == nil {
		t.Error("Connect found a cloud outside the override")
	}
	if auth.hits.Load() != 0 {
		t.Error("Connect authenticated with a non-override clouds.yaml")
	}
}
