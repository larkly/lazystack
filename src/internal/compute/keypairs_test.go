package compute

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// keypairsFixture is a minimal nova API paginated keypair list response.
const keypairsFixture = `{
  "keypairs": [
    {
      "keypair": {
        "name": "mykey",
        "type": "ssh",
        "public_key": "ssh-rsa AAAAB3NzaC1yc2E...",
        "fingerprint": "eb:0d:4e:0a:22:28:45:81:2e:07:5b:a4:04:f0:ac:97"
      }
    },
    {
      "keypair": {
        "name": "admin-key",
        "type": "x509",
        "public_key": "-----BEGIN CERTIFICATE-----...",
        "fingerprint": "fa:9b:3c:f1:11:37:54:92:3d:f6:4a:b3:05:e1:bd:86"
      }
    }
  ]
}`

func TestListKeyPairs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/os-keypairs") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(keypairsFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeNovaClient(handler)
	ctx := context.Background()

	keypairs, err := ListKeyPairs(ctx, client)
	if err != nil {
		t.Fatalf("ListKeyPairs() error: %v", err)
	}
	if len(keypairs) != 2 {
		t.Fatalf("expected 2 keypairs, got %d", len(keypairs))
	}

	kp1 := keypairs[0]
	if kp1.Name != "mykey" {
		t.Errorf("unexpected name: %s", kp1.Name)
	}
	if kp1.Type != "ssh" {
		t.Errorf("unexpected type: %s", kp1.Type)
	}

	kp2 := keypairs[1]
	if kp2.Name != "admin-key" {
		t.Errorf("unexpected second name: %s", kp2.Name)
	}
	if kp2.Type != "x509" {
		t.Errorf("unexpected second type: %s", kp2.Type)
	}
}
