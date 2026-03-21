package image

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// Image is a simplified representation of a Glance image.
type Image struct {
	ID     string
	Name   string
	Status string
	Size   int64 // bytes
	MinDisk int
	MinRAM  int
}

// ListImages fetches all active images.
func ListImages(ctx context.Context, client *gophercloud.ServiceClient) ([]Image, error) {
	opts := images.ListOpts{
		Status: "active",
		Sort:   "name:asc",
	}

	var result []Image
	err := images.List(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := images.ExtractImages(page)
		if err != nil {
			return false, err
		}
		for _, img := range extracted {
			result = append(result, Image{
				ID:      img.ID,
				Name:    img.Name,
				Status:  string(img.Status),
				Size:    img.SizeBytes,
				MinDisk: img.MinDiskGigabytes,
				MinRAM:  img.MinRAMMegabytes,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}
	return result, nil
}
