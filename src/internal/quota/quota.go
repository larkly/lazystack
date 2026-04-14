package quota

import (
	"context"
	"fmt"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/gophercloud/gophercloud/v2"
	computequotas "github.com/gophercloud/gophercloud/v2/openstack/compute/v2/quotasets"
	networkquotas "github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/quotas"
	bsquotas "github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/quotasets"
)

// QuotaUsage represents a single quota resource.
type QuotaUsage struct {
	Resource string
	Used     int
	Limit    int
}

// requireProjectID guards against empty project IDs, which would otherwise
// produce a malformed /os-quota-sets//detail URL at the OpenStack API layer.
func requireProjectID(projectID string) error {
	if projectID == "" {
		return fmt.Errorf("projectID is required")
	}
	return nil
}

// GetComputeQuotas returns compute quota usage.
func GetComputeQuotas(ctx context.Context, client *gophercloud.ServiceClient, projectID string) ([]QuotaUsage, error) {
	shared.Debugf("[quota] GetComputeQuotas: start projectID=%s", projectID)
	if err := requireProjectID(projectID); err != nil {
		shared.Debugf("[quota] GetComputeQuotas: %v", err)
		return nil, err
	}
	detail, err := computequotas.GetDetail(ctx, client, projectID).Extract()
	if err != nil {
		shared.Debugf("[quota] GetComputeQuotas: error: %v", err)
		return nil, fmt.Errorf("compute quotas: %w", err)
	}
	return []QuotaUsage{
		{Resource: "Instances", Used: detail.Instances.InUse, Limit: detail.Instances.Limit},
		{Resource: "Cores", Used: detail.Cores.InUse, Limit: detail.Cores.Limit},
		{Resource: "RAM (MB)", Used: detail.RAM.InUse, Limit: detail.RAM.Limit},
		{Resource: "Key Pairs", Used: detail.KeyPairs.InUse, Limit: detail.KeyPairs.Limit},
		{Resource: "Server Groups", Used: detail.ServerGroups.InUse, Limit: detail.ServerGroups.Limit},
	}, nil
}

// GetNetworkQuotas returns network quota usage.
func GetNetworkQuotas(ctx context.Context, client *gophercloud.ServiceClient, projectID string) ([]QuotaUsage, error) {
	shared.Debugf("[quota] GetNetworkQuotas: start projectID=%s", projectID)
	if err := requireProjectID(projectID); err != nil {
		shared.Debugf("[quota] GetNetworkQuotas: %v", err)
		return nil, err
	}
	detail, err := networkquotas.GetDetail(ctx, client, projectID).Extract()
	if err != nil {
		shared.Debugf("[quota] GetNetworkQuotas: error: %v", err)
		return nil, fmt.Errorf("network quotas: %w", err)
	}
	return []QuotaUsage{
		{Resource: "Floating IPs", Used: detail.FloatingIP.Used, Limit: detail.FloatingIP.Limit},
		{Resource: "Networks", Used: detail.Network.Used, Limit: detail.Network.Limit},
		{Resource: "Ports", Used: detail.Port.Used, Limit: detail.Port.Limit},
		{Resource: "Routers", Used: detail.Router.Used, Limit: detail.Router.Limit},
		{Resource: "Security Groups", Used: detail.SecurityGroup.Used, Limit: detail.SecurityGroup.Limit},
		{Resource: "Subnets", Used: detail.Subnet.Used, Limit: detail.Subnet.Limit},
	}, nil
}

// GetVolumeQuotas returns block storage quota usage.
func GetVolumeQuotas(ctx context.Context, client *gophercloud.ServiceClient, projectID string) ([]QuotaUsage, error) {
	shared.Debugf("[quota] GetVolumeQuotas: start projectID=%s", projectID)
	if err := requireProjectID(projectID); err != nil {
		shared.Debugf("[quota] GetVolumeQuotas: %v", err)
		return nil, err
	}
	usage, err := bsquotas.GetUsage(ctx, client, projectID).Extract()
	if err != nil {
		shared.Debugf("[quota] GetVolumeQuotas: error: %v", err)
		return nil, fmt.Errorf("volume quotas: %w", err)
	}
	return []QuotaUsage{
		{Resource: "Volumes", Used: usage.Volumes.InUse, Limit: usage.Volumes.Limit},
		{Resource: "Gigabytes", Used: usage.Gigabytes.InUse, Limit: usage.Gigabytes.Limit},
		{Resource: "Snapshots", Used: usage.Snapshots.InUse, Limit: usage.Snapshots.Limit},
		{Resource: "Backups", Used: usage.Backups.InUse, Limit: usage.Backups.Limit},
	}, nil
}
