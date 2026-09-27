package image

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
)

func TestImageFromGophercloud(t *testing.T) {
	img := images.Image{
		ID:               "img-123",
		Name:             "ubuntu-22.04",
		Status:           images.ImageStatusActive,
		SizeBytes:        2 << 30, // 2GB
		MinDiskGigabytes: 10,
		MinRAMMegabytes:  1024,
		Visibility:       images.ImageVisibilityPublic,
		DiskFormat:       "qcow2",
		ContainerFormat:  "bare",
		Tags:             []string{"ubuntu", "lts"},
		Checksum:         "abc123",
		Owner:            "project-xyz",
		Protected:        true,
	}

	result := imageFromGophercloud(img)

	if result.ID != "img-123" {
		t.Errorf("ID = %s, want img-123", result.ID)
	}
	if result.Name != "ubuntu-22.04" {
		t.Errorf("Name = %s, want ubuntu-22.04", result.Name)
	}
	if result.Status != "active" {
		t.Errorf("Status = %s, want active", result.Status)
	}
	if result.Size != 2<<30 {
		t.Errorf("Size = %d, want %d", result.Size, 2<<30)
	}
	if result.MinDisk != 10 {
		t.Errorf("MinDisk = %d, want 10", result.MinDisk)
	}
	if result.MinRAM != 1024 {
		t.Errorf("MinRAM = %d, want 1024", result.MinRAM)
	}
	if result.Visibility != "public" {
		t.Errorf("Visibility = %s, want public", result.Visibility)
	}
	if result.DiskFormat != "qcow2" {
		t.Errorf("DiskFormat = %s, want qcow2", result.DiskFormat)
	}
	if result.ContainerFormat != "bare" {
		t.Errorf("ContainerFormat = %s, want bare", result.ContainerFormat)
	}
	if len(result.Tags) != 2 || result.Tags[0] != "ubuntu" || result.Tags[1] != "lts" {
		t.Errorf("Tags = %v, want [ubuntu lts]", result.Tags)
	}
	if result.Checksum != "abc123" {
		t.Errorf("Checksum = %s, want abc123", result.Checksum)
	}
	if result.Owner != "project-xyz" {
		t.Errorf("Owner = %s, want project-xyz", result.Owner)
	}
	if !result.Protected {
		t.Error("Protected should be true")
	}
}

func TestImageFromGophercloud_Empty(t *testing.T) {
	img := images.Image{}
	result := imageFromGophercloud(img)

	// Should not panic, all zero values
	if result.ID != "" {
		t.Errorf("ID = %s, want empty string", result.ID)
	}
	if result.Status != "" {
		t.Errorf("Status = %s, want empty string", result.Status)
	}
	if result.Size != 0 {
		t.Errorf("Size = %d, want 0", result.Size)
	}
	if result.Protected {
		t.Error("Protected should be false for empty image")
	}
}

func TestImageFromGophercloud_QueuedStatus(t *testing.T) {
	img := images.Image{
		ID:     "img-queued",
		Status: images.ImageStatusQueued,
		Name:   "uploading-image",
	}

	result := imageFromGophercloud(img)
	if result.Status != "queued" {
		t.Errorf("Status = %s, want queued", result.Status)
	}
}

func TestImageFromGophercloud_PrivateVisibility(t *testing.T) {
	img := images.Image{
		ID:         "img-private",
		Visibility: images.ImageVisibilityPrivate,
	}

	result := imageFromGophercloud(img)
	if result.Visibility != "private" {
		t.Errorf("Visibility = %s, want private", result.Visibility)
	}
}

func TestImageFromGophercloud_SharedVisibility(t *testing.T) {
	img := images.Image{
		ID:         "img-shared",
		Visibility: images.ImageVisibilityShared,
	}

	result := imageFromGophercloud(img)
	if result.Visibility != "shared" {
		t.Errorf("Visibility = %s, want shared", result.Visibility)
	}
}

func TestImageFromGophercloud_CommunityVisibility(t *testing.T) {
	img := images.Image{
		ID:         "img-community",
		Visibility: images.ImageVisibilityCommunity,
	}

	result := imageFromGophercloud(img)
	if result.Visibility != "community" {
		t.Errorf("Visibility = %s, want community", result.Visibility)
	}
}

// imageListFixture is a Glance v2 image list response.
const imageListFixture = `{
  "images": [
    {
      "id": "8a8a2d36-3f39-4e3a-b3da-2e4a4f2c3f61",
      "name": "ubuntu-22.04",
      "status": "active",
      "visibility": "public",
      "disk_format": "qcow2",
      "container_format": "bare",
      "size": 2147483648,
      "min_disk": 10,
      "min_ram": 1024,
      "protected": true,
      "tags": ["ubuntu", "lts"],
      "checksum": "d41d8cd98f00b204e9800998ecf8427e",
      "owner": "c57f2d4d1b7e46c6a1f5",
      "created_at": "2026-01-15T08:30:00Z",
      "updated_at": "2026-02-20T14:22:00Z"
    },
    {
      "id": "3e1f5b9a-d82f-4c12-9a7b-f812c44d8b3a",
      "name": "debian-12",
      "status": "saving",
      "visibility": "private",
      "disk_format": "raw",
      "container_format": "bare",
      "size": 10737418240,
      "min_disk": 20,
      "min_ram": 2048,
      "protected": false,
      "tags": ["debian"],
      "checksum": "",
      "owner": "c57f2d4d1b7e46c6a1f5",
      "created_at": "2026-04-01T12:00:00Z",
      "updated_at": "2026-04-01T12:00:00Z"
    }
  ]
}`

// imageDetailFixture is a single image detail response (from GET /v2/images/{id}).
const imageDetailFixture = `{
  "id": "8a8a2d36-3f39-4e3a-b3da-2e4a4f2c3f61",
  "name": "ubuntu-22.04",
  "status": "active",
  "visibility": "public",
  "disk_format": "qcow2",
  "container_format": "bare",
  "size": 2147483648,
  "min_disk": 10,
  "min_ram": 1024,
  "protected": true,
  "tags": ["ubuntu", "lts"],
  "checksum": "d41d8cd98f00b204e9800998ecf8427e",
  "owner": "c57f2d4d1b7e46c6a1f5",
  "created_at": "2026-01-15T08:30:00Z",
  "updated_at": "2026-02-20T14:22:00Z"
}`

func fakeGlanceClient(handler http.Handler) *gophercloud.ServiceClient {
	srv := httptest.NewServer(handler)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			HTTPClient: *srv.Client(),
		},
		Endpoint: srv.URL + "/",
	}
}

func TestListImages(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/images") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(imageListFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeGlanceClient(handler)
	ctx := context.Background()

	imgs, err := ListImages(ctx, client)
	if err != nil {
		t.Fatalf("ListImages() error: %v", err)
	}
	if len(imgs) != 2 {
		t.Fatalf("expected 2 images, got %d", len(imgs))
	}

	// Verify first image (active)
	i1 := imgs[0]
	if i1.ID != "8a8a2d36-3f39-4e3a-b3da-2e4a4f2c3f61" {
		t.Errorf("unexpected ID: %s", i1.ID)
	}
	if i1.Name != "ubuntu-22.04" {
		t.Errorf("unexpected Name: %s", i1.Name)
	}
	if i1.Status != "active" {
		t.Errorf("unexpected Status: %s", i1.Status)
	}
	if i1.Visibility != "public" {
		t.Errorf("unexpected Visibility: %s", i1.Visibility)
	}
	if i1.DiskFormat != "qcow2" {
		t.Errorf("unexpected DiskFormat: %s", i1.DiskFormat)
	}
	if i1.ContainerFormat != "bare" {
		t.Errorf("unexpected ContainerFormat: %s", i1.ContainerFormat)
	}
	if i1.Size != 2147483648 {
		t.Errorf("unexpected Size: %d", i1.Size)
	}
	if i1.MinDisk != 10 {
		t.Errorf("unexpected MinDisk: %d", i1.MinDisk)
	}
	if i1.MinRAM != 1024 {
		t.Errorf("unexpected MinRAM: %d", i1.MinRAM)
	}
	if !i1.Protected {
		t.Error("expected Protected to be true")
	}

	// Verify second image (saving)
	i2 := imgs[1]
	if i2.ID != "3e1f5b9a-d82f-4c12-9a7b-f812c44d8b3a" {
		t.Errorf("unexpected ID: %s", i2.ID)
	}
	if i2.Name != "debian-12" {
		t.Errorf("unexpected Name: %s", i2.Name)
	}
	if i2.Status != "saving" {
		t.Errorf("unexpected Status: %s", i2.Status)
	}
	if i2.Visibility != "private" {
		t.Errorf("unexpected Visibility: %s", i2.Visibility)
	}
	if i2.Protected {
		t.Error("expected Protected to be false")
	}
}

func TestGetImage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/images/8a8a2d36") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(imageDetailFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeGlanceClient(handler)
	ctx := context.Background()

	img, err := GetImage(ctx, client, "8a8a2d36-3f39-4e3a-b3da-2e4a4f2c3f61")
	if err != nil {
		t.Fatalf("GetImage() error: %v", err)
	}

	if img.ID != "8a8a2d36-3f39-4e3a-b3da-2e4a4f2c3f61" {
		t.Errorf("unexpected ID: %s", img.ID)
	}
	if img.Name != "ubuntu-22.04" {
		t.Errorf("unexpected Name: %s", img.Name)
	}
	if img.Status != "active" {
		t.Errorf("unexpected Status: %s", img.Status)
	}
	if img.Visibility != "public" {
		t.Errorf("unexpected Visibility: %s", img.Visibility)
	}
	if img.DiskFormat != "qcow2" {
		t.Errorf("unexpected DiskFormat: %s", img.DiskFormat)
	}
	if img.Size != 2147483648 {
		t.Errorf("unexpected Size: %d", img.Size)
	}
	if img.MinDisk != 10 {
		t.Errorf("unexpected MinDisk: %d", img.MinDisk)
	}
	if img.MinRAM != 1024 {
		t.Errorf("unexpected MinRAM: %d", img.MinRAM)
	}
	if !img.Protected {
		t.Error("expected Protected to be true")
	}
	if img.Checksum != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("unexpected Checksum: %s", img.Checksum)
	}
}

func TestDeleteImage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/images/img-to-delete") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeGlanceClient(handler)
	ctx := context.Background()

	err := DeleteImage(ctx, client, "img-to-delete")
	if err != nil {
		t.Fatalf("DeleteImage() error: %v", err)
	}
}
