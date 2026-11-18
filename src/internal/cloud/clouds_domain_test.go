package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// tokenRecorder is a keystone stub that keeps the last token request body
// and rejects it, so Connect stops right after authentication.
type tokenRecorder struct {
	srv  *httptest.Server
	hits atomic.Int32
	body atomic.Value
}

func newTokenRecorder(t *testing.T) *tokenRecorder {
	t.Helper()
	r := &tokenRecorder{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits.Add(1)
		var body map[string]any
		_ = json.NewDecoder(req.Body).Decode(&body)
		r.body.Store(body)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *tokenRecorder) url() string { return r.srv.URL + "/v3" }

// domain returns the domain object found under path in the last token
// request, as "id=..." or "name=...", or "" when there is none.
func (r *tokenRecorder) domain(path ...string) string {
	var cur any = r.body.Load()
	for _, key := range append(path, "domain") {
		m, _ := cur.(map[string]any)
		cur = m[key]
	}
	d, _ := cur.(map[string]any)
	if id, _ := d["id"].(string); id != "" {
		return "id=" + id
	}
	if name, _ := d["name"].(string); name != "" {
		return "name=" + name
	}
	return ""
}

func (r *tokenRecorder) userDomain() string {
	return r.domain("auth", "identity", "password", "user")
}

func (r *tokenRecorder) projectDomain() string {
	return r.domain("auth", "scope", "project")
}

// A clouds.yaml that names only the user's domain or only the project's
// must keep authenticating, with the missing one inherited from the other.
func TestConnectInheritsMissingDomain(t *testing.T) {
	for _, tc := range []struct {
		name          string
		auth          string // extra auth keys, one per line
		userDomain    string
		projectDomain string // "" when the project is scoped by ID
	}{
		{
			name:          "project by name inherits the user domain name",
			auth:          "project_name: p\nuser_domain_name: Default",
			userDomain:    "name=Default",
			projectDomain: "name=Default",
		},
		{
			name:          "project by name inherits the user domain ID",
			auth:          "project_name: p\nuser_domain_id: default",
			userDomain:    "id=default",
			projectDomain: "id=default",
		},
		{
			name:          "user inherits the domain of a project scoped by name",
			auth:          "project_name: p\nproject_domain_name: Default",
			userDomain:    "name=Default",
			projectDomain: "name=Default",
		},
		{
			name:       "user inherits the domain of a project scoped by ID",
			auth:       "project_id: abc123\nproject_domain_id: default",
			userDomain: "id=default",
		},
		{
			name:          "separate domains are left alone",
			auth:          "project_name: p\nuser_domain_name: Users\nproject_domain_name: Projects",
			userDomain:    "name=Users",
			projectDomain: "name=Projects",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newCloudsEnv(t)
			rec := newTokenRecorder(t)
			content := "clouds:\n  mycloud:\n    auth:\n      auth_url: " + rec.url() +
				"\n      username: u\n      password: pw\n"
			for _, line := range strings.Split(tc.auth, "\n") {
				content += "      " + line + "\n"
			}
			writeFile(t, filepath.Join(e.home, ".config", "openstack", "clouds.yaml"), content)

			if _, err := Connect(context.Background(), "mycloud"); err == nil {
				t.Fatal("expected the stub's 401 to fail Connect")
			}
			if rec.hits.Load() == 0 {
				t.Fatal("Connect never sent a token request")
			}
			if got := rec.userDomain(); got != tc.userDomain {
				t.Errorf("user domain = %q, want %q", got, tc.userDomain)
			}
			if got := rec.projectDomain(); got != tc.projectDomain {
				t.Errorf("project domain = %q, want %q", got, tc.projectDomain)
			}
		})
	}
}

// A cloud with a profile: key must connect without a clouds-public.yaml, and
// a clouds-public.yaml planted in the working directory must not be able to
// supply the auth URL.
func TestConnectIgnoresCloudsPublicYaml(t *testing.T) {
	const publicYAML = "public-clouds:\n  vendor:\n    auth:\n      auth_url: "
	cloudWithProfile := func(authURL string) string {
		c := "clouds:\n  mycloud:\n    profile: vendor\n    auth:\n"
		if authURL != "" {
			c += "      auth_url: " + authURL + "\n"
		}
		return c + "      username: u\n      password: pw\n      project_name: p\n      user_domain_name: Default\n      project_domain_name: Default\n"
	}

	t.Run("profile without clouds-public.yaml still connects", func(t *testing.T) {
		e := newCloudsEnv(t)
		rec := newTokenRecorder(t)
		writeFile(t, filepath.Join(e.home, ".config", "openstack", "clouds.yaml"), cloudWithProfile(rec.url()))

		if _, err := Connect(context.Background(), "mycloud"); err == nil {
			t.Fatal("expected the stub's 401 to fail Connect")
		}
		if rec.hits.Load() == 0 {
			t.Error("Connect never sent a token request for a cloud with a profile")
		}
	})

	t.Run("planted clouds-public.yaml does not supply the auth URL", func(t *testing.T) {
		e := newCloudsEnv(t)
		planted := newTokenRecorder(t)
		writeFile(t, filepath.Join(e.home, ".config", "openstack", "clouds.yaml"), cloudWithProfile(""))
		writeFile(t, filepath.Join(e.cwd, "clouds-public.yaml"), publicYAML+planted.url()+"\n")

		if _, err := Connect(context.Background(), "mycloud"); err == nil {
			t.Fatal("Connect succeeded without an auth URL")
		}
		if planted.hits.Load() != 0 {
			t.Error("Connect sent credentials to the auth URL from clouds-public.yaml")
		}
	})
}
