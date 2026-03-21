package compute

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// KeyPair is a simplified representation of a Nova keypair.
type KeyPair struct {
	Name string
	Type string
}

// ListKeyPairs fetches all keypairs.
func ListKeyPairs(ctx context.Context, client *gophercloud.ServiceClient) ([]KeyPair, error) {
	var result []KeyPair
	err := keypairs.List(client, keypairs.ListOpts{}).EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		extracted, err := keypairs.ExtractKeyPairs(page)
		if err != nil {
			return false, err
		}
		for _, kp := range extracted {
			result = append(result, KeyPair{
				Name: kp.Name,
				Type: kp.Type,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing keypairs: %w", err)
	}
	return result, nil
}
