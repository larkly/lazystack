package compute

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/larkly/lazystack/internal/testutil"
)

func TestCreateSnapshotConflictReasons(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		want    string
		notWant string
	}{
		{"locked", `{"conflictingRequest":{"code":409,"message":"Instance srv is locked"}}`, "Instance srv is locked", "snapshot in progress"},
		{"invalid state", `{"conflictingRequest":{"code":409,"message":"Cannot 'createImage' instance srv while it is in vm_state error"}}`, "vm_state error", "snapshot in progress"},
		{"no body", ``, "cannot be snapshotted in its current state", "snapshot in progress"},
		{"snapshot running", `{"conflictingRequest":{"code":409,"message":"Cannot 'createImage' instance srv while it is in task_state image_snapshot"}}`, "snapshot in progress", ""},
		{"upload pending", `{"conflictingRequest":{"code":409,"message":"Cannot 'createImage' instance srv while it is in task_state image_pending_upload"}}`, "snapshot in progress", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/servers/srv/action" {
					t.Errorf("request=%s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				w.Write([]byte(tc.body))
			}))
			defer close()
			err := CreateSnapshot(context.Background(), client, "srv", "snap")
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err=%q, want it to contain %q", err, tc.want)
			}
			if tc.notWant != "" && strings.Contains(err.Error(), tc.notWant) {
				t.Errorf("err=%q must not contain %q", err, tc.notWant)
			}
			if !gophercloud.ResponseCodeIs(err, http.StatusConflict) {
				t.Errorf("HTTP 409 cause not inspectable through %T", err)
			}
		})
	}
}

func TestGetPasswordHTTP(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// Nova encrypts the admin password with RSA PKCS#1 v1.5, so the fixture
	// must produce that legacy ciphertext for the decryption path to be tested.
	//lint:ignore SA1019 Nova-compatible PKCS#1 v1.5 ciphertext is the test contract
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, &key.PublicKey, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	encrypted := base64.StdEncoding.EncodeToString(ciphertext)
	path := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, password, key, plain, wantErr string
		status                              int
	}{
		{"decrypt", encrypted, path, "secret", "", 200}, {"encrypted only", encrypted, "", "", "", 200}, {"empty", "", path, "", "", 200},
		{"missing key", encrypted, path + "missing", "", "reading key file", 200}, {"bad ciphertext", "not-base64", path, "", "decrypting password", 200},
		{"api error", "", "", "", "getting password for vm", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/servers/vm/os-server-password" {
					t.Errorf("request %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				json.NewEncoder(w).Encode(map[string]string{"password": tc.password})
			}))
			defer close()
			plain, enc, err := GetPassword(context.Background(), client, "vm", tc.key)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if plain != tc.plain || (tc.status == 200 && enc != tc.password) {
				t.Fatalf("plain=%q encrypted=%q", plain, enc)
			}
		})
	}
}

func TestRescueAndEvacuateHTTP(t *testing.T) {
	for _, action := range []string{"rescue", "evacuate"} {
		for _, tc := range []struct {
			name, body string
			status     int
			want       string
			fail       bool
		}{
			{"password", `{"adminPass":"generated"}`, 200, "generated", false}, {"absent", `{}`, 200, "", false}, {"failure", `{}`, 409, "", true},
		} {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != "POST" || r.URL.Path != "/servers/vm/action" {
						t.Errorf("request %s %s", r.Method, r.URL.Path)
					}
					var body map[string]map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					opts, ok := body[action]
					if !ok {
						t.Errorf("missing action: %v", body)
					}
					if action == "evacuate" && (opts["host"] != "target" || opts["onSharedStorage"] != true) {
						t.Errorf("opts=%v", opts)
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.status)
					w.Write([]byte(tc.body))
				}))
				defer close()
				var got string
				var err error
				if action == "rescue" {
					got, err = RescueServer(context.Background(), client, "vm")
				} else {
					got, err = EvacuateServer(context.Background(), client, "vm", "target", true)
				}
				if (err != nil) != tc.fail || got != tc.want {
					t.Fatalf("got %q err=%v", got, err)
				}
			})
		}
	}
}

type extendedServerOpts struct{ servers.CreateOpts }

func (o extendedServerOpts) ToServerCreateMap() (map[string]any, error) {
	m, err := o.CreateOpts.ToServerCreateMap()
	if err == nil {
		m["server"].(map[string]any)["key_name"] = "test-key"
	}
	return m, err
}
func TestCreateServerWithOptsHTTP(t *testing.T) {
	for _, status := range []int{202, 400} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/servers" {
					t.Errorf("request %s %s", r.Method, r.URL.Path)
				}
				var body map[string]map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				s := body["server"]
				if s["name"] != "vm" || s["imageRef"] != "img" || s["flavorRef"] != "flavor" || s["key_name"] != "test-key" {
					t.Errorf("body=%v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				w.Write([]byte(`{"server":{"id":"new","name":"vm","status":"BUILD","key_name":"test-key","metadata":{"role":"web"}}}`))
			}))
			defer close()
			got, err := CreateServerWithOpts(context.Background(), client, extendedServerOpts{servers.CreateOpts{Name: "vm", ImageRef: "img", FlavorRef: "flavor"}})
			if status == 400 {
				if err == nil || got != nil || !strings.Contains(err.Error(), "creating server") {
					t.Fatalf("got=%v err=%v", got, err)
				}
				return
			}
			if err != nil || got.ID != "new" || got.KeyName != "test-key" || got.Metadata["role"] != "web" {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}
