package image

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
)

func TestProgressReader(t *testing.T) {
	data := []byte("hello world")
	pr := &ProgressReader{
		Reader: bytes.NewReader(data),
		Total:  int64(len(data)),
	}

	// Read in chunks — bytes.Reader may return fewer bytes than buffer size
	totalRead := int64(0)
	buf := make([]byte, 5)
	for {
		n, err := pr.Read(buf)
		if n > 0 {
			// Verify each chunk is a substring of the original data
			expectedStart := string(data[totalRead : totalRead+int64(n)])
			got := string(buf[:n])
			if got != expectedStart {
				t.Errorf("got %q, want %q at offset %d", got, expectedStart, totalRead)
			}
		}
		totalRead += int64(n)
		if err != nil {
			break // io.EOF or other error
		}
	}

	if totalRead != 11 {
		t.Errorf("total bytes read = %d, want 11", totalRead)
	}
	if pr.BytesRead() != 11 {
		t.Errorf("BytesRead = %d, want 11", pr.BytesRead())
	}
}

func TestProgressReader_LargeData(t *testing.T) {
	// Test with larger data to verify atomic counting
	size := 10000
	data := bytes.Repeat([]byte("x"), size)
	pr := &ProgressReader{
		Reader: bytes.NewReader(data),
		Total:  int64(size),
	}

	// Read in small chunks
	buf := make([]byte, 37) // prime number to avoid alignment
	totalRead := int64(0)
	for {
		n, err := pr.Read(buf)
		totalRead += int64(n)
		if err != nil {
			break
		}
	}

	if totalRead != int64(size) {
		t.Errorf("total bytes read = %d, want %d", totalRead, size)
	}
	if pr.BytesRead() != totalRead {
		t.Errorf("BytesRead = %d, want %d", pr.BytesRead(), totalRead)
	}
}

func TestProgressReader_ZeroBytesRead(t *testing.T) {
	pr := &ProgressReader{
		Reader: strings.NewReader(""),
	}

	n, _ := pr.Read(make([]byte, 1))
	if n != 0 {
		t.Errorf("expected n=0, got %d", n)
	}
	if pr.BytesRead() != 0 {
		t.Errorf("BytesRead should be 0, got %d", pr.BytesRead())
	}
}

func TestProgressReader_ConcurrentAccess(t *testing.T) {
	// Verify atomic safety
	pr := &ProgressReader{
		Reader: bytes.NewReader(bytes.Repeat([]byte("a"), 1000)),
		Total:  1000,
	}

	done := make(chan bool)
	go func() {
		buf := make([]byte, 1)
		for {
			_, err := pr.Read(buf)
			if err != nil {
				break
			}
		}
		done <- true
	}()

	// Read BytesRead concurrently while goroutine is running
	for i := 0; i < 100; i++ {
		_ = pr.BytesRead() // Just verify it doesn't panic
	}

	<-done

	if pr.BytesRead() != 1000 {
		t.Errorf("BytesRead = %d, want 1000", pr.BytesRead())
	}
}

func TestImageFromGophercloud(t *testing.T) {
	img := images.Image{
		ID:                 "img-123",
		Name:               "ubuntu-22.04",
		Status:             images.ImageStatusActive,
		SizeBytes:          2 << 30, // 2GB
		MinDiskGigabytes:   10,
		MinRAMMegabytes:    1024,
		Visibility:         images.ImageVisibilityPublic,
		DiskFormat:         "qcow2",
		ContainerFormat:    "bare",
		Tags:               []string{"ubuntu", "lts"},
		Checksum:           "abc123",
		Owner:              "project-xyz",
		Protected:          true,
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
