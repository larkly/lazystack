package image

import (
	"context"
	"fmt"
	"time"

	"github.com/gophercloud/gophercloud/v2"
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
	if len(patches) == 0 {
		return nil
	}
	_, err := images.Update(ctx, client, id, patches).Extract()
	if err != nil {
		return fmt.Errorf("updating image %s: %w", id, err)
	}
	return nil
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
