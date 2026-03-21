package cloud

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/config"
	"github.com/gophercloud/gophercloud/v2/openstack/config/clouds"
)

// Client holds authenticated OpenStack service clients.
type Client struct {
	CloudName    string
	Region       string
	Compute      *gophercloud.ServiceClient
	Image        *gophercloud.ServiceClient
	Network      *gophercloud.ServiceClient
	BlockStorage *gophercloud.ServiceClient
}

// Connect authenticates to the given cloud and initializes service clients.
func Connect(ctx context.Context, cloudName string) (*Client, error) {
	ao, eo, tlsConfig, err := clouds.Parse(clouds.WithCloudName(cloudName))
	if err != nil {
		return nil, fmt.Errorf("parsing cloud %q: %w", cloudName, err)
	}

	providerClient, err := config.NewProviderClient(ctx, ao, config.WithTLSConfig(tlsConfig))
	if err != nil {
		return nil, fmt.Errorf("authenticating to %q: %w", cloudName, err)
	}

	compute, err := openstack.NewComputeV2(providerClient, eo)
	if err != nil {
		return nil, fmt.Errorf("compute client: %w", err)
	}
	compute.Microversion = "2.100"

	image, err := openstack.NewImageV2(providerClient, eo)
	if err != nil {
		return nil, fmt.Errorf("image client: %w", err)
	}

	network, err := openstack.NewNetworkV2(providerClient, eo)
	if err != nil {
		return nil, fmt.Errorf("network client: %w", err)
	}

	// BlockStorage — try v3 first ("block-storage"), then v2, then v1 ("volume")
	// Different clouds register Cinder under different service types
	blockStorage := tryBlockStorage(providerClient, eo)

	region := eo.Region
	if region == "" {
		region = "default"
	}

	return &Client{
		CloudName:    cloudName,
		Region:       region,
		Compute:      compute,
		Image:        image,
		Network:      network,
		BlockStorage: blockStorage,
	}, nil
}

func tryBlockStorage(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) *gophercloud.ServiceClient {
	// Try the standard v3 service type
	if sc, err := openstack.NewBlockStorageV3(pc, eo); err == nil {
		return sc
	}
	// Try v2
	if sc, err := openstack.NewBlockStorageV2(pc, eo); err == nil {
		return sc
	}
	// Try v1 (service type "volume")
	if sc, err := openstack.NewBlockStorageV1(pc, eo); err == nil {
		return sc
	}
	return nil
}
