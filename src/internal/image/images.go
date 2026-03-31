package image

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/imagedata"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/imageimport"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// Image is a simplified representation of a Glance image.
type Image struct {
	ID              string
	Name            string
	Status          string
	Size            int64 // bytes
	MinDisk         int
	MinRAM          int
	Visibility      string
	DiskFormat      string
	ContainerFormat string
	Tags            []string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Checksum        string
	Owner           string
	Protected       bool
}

// ListImages fetches all images (all statuses).
func ListImages(ctx context.Context, client *gophercloud.ServiceClient) ([]Image, error) {
	opts := images.ListOpts{
		Sort: "name:asc",
	}

	var result []Image
	err := images.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := images.ExtractImages(page)
		if err != nil {
			return false, err
		}
		for _, img := range extracted {
			result = append(result, imageFromGophercloud(img))
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}
	return result, nil
}

// GetImage fetches a single image by ID.
func GetImage(ctx context.Context, client *gophercloud.ServiceClient, id string) (*Image, error) {
	raw, err := images.Get(ctx, client, id).Extract()
	if err != nil {
		return nil, fmt.Errorf("getting image %s: %w", id, err)
	}
	img := imageFromGophercloud(*raw)
	return &img, nil
}

// DeleteImage deletes an image by ID.
func DeleteImage(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	err := images.Delete(ctx, client, id).ExtractErr()
	if err != nil {
		return fmt.Errorf("deleting image %s: %w", id, err)
	}
	return nil
}

// UpdateImageOpts holds optional fields for updating an image.
type UpdateImageOpts struct {
	Name       *string
	Visibility *string
	MinDisk    *int
	MinRAM     *int
	Tags       *[]string
	Protected  *bool
}

// UpdateImage updates image properties.
func UpdateImage(ctx context.Context, client *gophercloud.ServiceClient, id string, opts UpdateImageOpts) error {
	var patches images.UpdateOpts
	if opts.Name != nil {
		patches = append(patches, images.ReplaceImageName{NewName: *opts.Name})
	}
	if opts.Visibility != nil {
		vis := images.ImageVisibility(*opts.Visibility)
		patches = append(patches, images.UpdateVisibility{Visibility: vis})
	}
	if opts.MinDisk != nil {
		patches = append(patches, images.ReplaceImageMinDisk{NewMinDisk: *opts.MinDisk})
	}
	if opts.MinRAM != nil {
		patches = append(patches, images.ReplaceImageMinRam{NewMinRam: *opts.MinRAM})
	}
	if opts.Tags != nil {
		patches = append(patches, images.ReplaceImageTags{NewTags: *opts.Tags})
	}
	if opts.Protected != nil {
		patches = append(patches, images.ReplaceImageProtected{NewProtected: *opts.Protected})
	}
	if len(patches) == 0 {
		return nil
	}
	_, err := images.Update(ctx, client, id, patches).Extract()
	if err != nil {
		return fmt.Errorf("updating image %s: %w", id, err)
	}
	return nil
}

// CreateImageOpts holds fields for creating a new image (metadata only).
type CreateImageOpts struct {
	Name            string
	DiskFormat      string
	ContainerFormat string
	Visibility      string
	MinDisk         int
	MinRAM          int
	Tags            []string
}

// CreateImage creates image metadata (status becomes "queued").
func CreateImage(ctx context.Context, client *gophercloud.ServiceClient, opts CreateImageOpts) (*Image, error) {
	containerFormat := opts.ContainerFormat
	if containerFormat == "" {
		containerFormat = "bare"
	}
	visibility := opts.Visibility
	if visibility == "" {
		visibility = "private"
	}
	vis := images.ImageVisibility(visibility)

	createOpts := images.CreateOpts{
		Name:            opts.Name,
		DiskFormat:      opts.DiskFormat,
		ContainerFormat: containerFormat,
		Visibility:      &vis,
		MinDisk:         opts.MinDisk,
		MinRAM:          opts.MinRAM,
		Tags:            opts.Tags,
	}

	raw, err := images.Create(ctx, client, createOpts).Extract()
	if err != nil {
		return nil, fmt.Errorf("creating image: %w", err)
	}
	img := imageFromGophercloud(*raw)
	return &img, nil
}

// UploadImageData uploads image file data to an existing image.
func UploadImageData(ctx context.Context, client *gophercloud.ServiceClient, imageID string, data io.Reader) error {
	err := imagedata.Upload(ctx, client, imageID, data).ExtractErr()
	if err != nil {
		return fmt.Errorf("uploading image data %s: %w", imageID, err)
	}
	return nil
}

// DownloadImageData downloads image file data. Returns a reader and content length (-1 if unknown).
func DownloadImageData(ctx context.Context, client *gophercloud.ServiceClient, imageID string) (io.ReadCloser, int64, error) {
	result := imagedata.Download(ctx, client, imageID)
	body, err := result.Extract()
	if err != nil {
		return nil, 0, fmt.Errorf("downloading image data %s: %w", imageID, err)
	}
	var contentLength int64 = -1
	if cl := result.Header.Get("Content-Length"); cl != "" {
		fmt.Sscanf(cl, "%d", &contentLength)
	}
	return body, contentLength, nil
}

// ImportImageURL triggers a web-download import for an image.
func ImportImageURL(ctx context.Context, client *gophercloud.ServiceClient, imageID, url string) error {
	opts := imageimport.CreateOpts{
		Name: imageimport.WebDownloadMethod,
		URI:  url,
	}
	err := imageimport.Create(ctx, client, imageID, opts).ExtractErr()
	if err != nil {
		return fmt.Errorf("importing image %s from URL: %w", imageID, err)
	}
	return nil
}

// ProgressReader wraps an io.Reader to track bytes read atomically.
type ProgressReader struct {
	Reader    io.Reader
	Total     int64
	bytesRead atomic.Int64
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.bytesRead.Add(int64(n))
	return n, err
}

// BytesRead returns the current number of bytes read (safe for concurrent access).
func (pr *ProgressReader) BytesRead() int64 {
	return pr.bytesRead.Load()
}

// DeactivateImage deactivates an image (prevents downloads).
func DeactivateImage(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	url := client.ServiceURL("images", id, "actions", "deactivate")
	resp, err := client.Post(ctx, url, nil, nil, &gophercloud.RequestOpts{
		OkCodes: []int{204},
	})
	if resp != nil {
		resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("deactivating image %s: %w", id, err)
	}
	return nil
}

// ReactivateImage reactivates a deactivated image.
func ReactivateImage(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	url := client.ServiceURL("images", id, "actions", "reactivate")
	resp, err := client.Post(ctx, url, nil, nil, &gophercloud.RequestOpts{
		OkCodes: []int{204},
	})
	if resp != nil {
		resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("reactivating image %s: %w", id, err)
	}
	return nil
}

func imageFromGophercloud(img images.Image) Image {
	return Image{
		ID:              img.ID,
		Name:            img.Name,
		Status:          string(img.Status),
		Size:            img.SizeBytes,
		MinDisk:         img.MinDiskGigabytes,
		MinRAM:          img.MinRAMMegabytes,
		Visibility:      string(img.Visibility),
		DiskFormat:      img.DiskFormat,
		ContainerFormat: img.ContainerFormat,
		Tags:            img.Tags,
		CreatedAt:       img.CreatedAt,
		UpdatedAt:       img.UpdatedAt,
		Checksum:        img.Checksum,
		Owner:           img.Owner,
		Protected:       img.Protected,
	}
}
