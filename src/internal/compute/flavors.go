package compute

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/larkly/lazystack/internal/shared"
)

// Flavor is a simplified representation of a Nova flavor.
type Flavor struct {
	ID    string
	Name  string
	VCPUs int
	RAM   int // MB
	Disk  int // GB
}

// ListFlavors fetches all available flavors.
func ListFlavors(ctx context.Context, client *gophercloud.ServiceClient) ([]Flavor, error) {
	shared.Debugf("[compute] listing flavors")
	opts := flavors.ListOpts{}

	var result []Flavor
	err := flavors.ListDetail(client, opts).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := flavors.ExtractFlavors(page)
		if err != nil {
			return false, err
		}
		for _, f := range extracted {
			result = append(result, Flavor{
				ID:    f.ID,
				Name:  f.Name,
				VCPUs: f.VCPUs,
				RAM:   f.RAM,
				Disk:  f.Disk,
			})
		}
		return true, nil
	})
	if err != nil {
		shared.Debugf("[compute] list flavors: %v", err)
		return nil, fmt.Errorf("listing flavors: %w", err)
	}
	shared.Debugf("[compute] listed %d flavors", len(result))
	return result, nil
}
