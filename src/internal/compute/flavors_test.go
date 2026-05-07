package compute

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

// flavorsFixture is a minimal nova API paginated flavor list response.
const flavorsFixture = `{
  "flavors": [
    {
      "id": "1",
      "name": "m1.tiny",
      "vcpus": 1,
      "ram": 512,
      "disk": 1
    },
    {
      "id": "2",
      "name": "m1.small",
      "vcpus": 1,
      "ram": 2048,
      "disk": 20
    },
    {
      "id": "3",
      "name": "m1.large",
      "vcpus": 4,
      "ram": 8192,
      "disk": 80
    }
  ]
}`

func fakeNovaClient(handler http.Handler) *gophercloud.ServiceClient {
	srv := httptest.NewServer(handler)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			HTTPClient: *srv.Client(),
		},
		Endpoint: srv.URL + "/",
	}
}

func TestListFlavors(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/flavors/detail") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(flavorsFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeNovaClient(handler)
	ctx := context.Background()

	flavors, err := ListFlavors(ctx, client)
	if err != nil {
		t.Fatalf("ListFlavors() error: %v", err)
	}
	if len(flavors) != 3 {
		t.Fatalf("expected 3 flavors, got %d", len(flavors))
	}

	// Verify m1.tiny
	f1 := flavors[0]
	if f1.ID != "1" {
		t.Errorf("unexpected ID: %s", f1.ID)
	}
	if f1.Name != "m1.tiny" {
		t.Errorf("unexpected Name: %s", f1.Name)
	}
	if f1.VCPUs != 1 {
		t.Errorf("unexpected VCPUs: %d", f1.VCPUs)
	}
	if f1.RAM != 512 {
		t.Errorf("unexpected RAM: %d", f1.RAM)
	}
	if f1.Disk != 1 {
		t.Errorf("unexpected Disk: %d", f1.Disk)
	}

	// Verify m1.large
	f3 := flavors[2]
	if f3.Name != "m1.large" {
		t.Errorf("unexpected third flavor name: %s", f3.Name)
	}
	if f3.VCPUs != 4 {
		t.Errorf("unexpected VCPUs for m1.large: %d", f3.VCPUs)
	}
	if f3.RAM != 8192 {
		t.Errorf("unexpected RAM for m1.large: %d", f3.RAM)
	}
	if f3.Disk != 80 {
		t.Errorf("unexpected Disk for m1.large: %d", f3.Disk)
	}
}
