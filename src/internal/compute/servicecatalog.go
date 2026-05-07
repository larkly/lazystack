package compute

import (
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
)

// EndpointInfo is a simplified endpoint representation.
type EndpointInfo struct {
	Interface string
	URL       string
	Region    string
}

// ServiceEntry is a simplified service catalog entry for UI display.
type ServiceEntry struct {
	Name      string
	Type      string
	Available bool
	Endpoints []EndpointInfo
}

// knownServiceType enumerates OpenStack services we know about.
type knownService struct {
	Type    string
	Name    string
	NewFunc func(*gophercloud.ProviderClient, gophercloud.EndpointOpts) (*gophercloud.ServiceClient, error)
}

var knownServices = []knownService{
	{Type: "compute", Name: "Compute (Nova)", NewFunc: openstack.NewComputeV2},
	{Type: "image", Name: "Image (Glance)", NewFunc: openstack.NewImageV2},
	{Type: "network", Name: "Network (Neutron)", NewFunc: openstack.NewNetworkV2},
	{Type: "identity", Name: "Identity (Keystone)", NewFunc: openstack.NewIdentityV3},
	{Type: "block-storage", Name: "Block Storage (Cinder)", NewFunc: func(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) (*gophercloud.ServiceClient, error) {
		sc, err := openstack.NewBlockStorageV3(pc, eo)
		if err != nil {
			sc, err = openstack.NewBlockStorageV2(pc, eo)
		}
		return sc, err
	}},
	{Type: "load-balancer", Name: "Load Balancer (Octavia)", NewFunc: openstack.NewLoadBalancerV2},
	{Type: "placement", Name: "Placement", NewFunc: openstack.NewPlacementV1},
	{Type: "dns", Name: "DNS (Designate)", NewFunc: openstack.NewDNSV2},
	{Type: "sharev2", Name: "Shared File Systems (Manila)", NewFunc: openstack.NewSharedFileSystemV2},
	{Type: "baremetal", Name: "Bare Metal (Ironic)", NewFunc: openstack.NewBareMetalV1},
	{Type: "container-infra", Name: "Container Infra (Magnum)", NewFunc: openstack.NewContainerInfraV1},
	{Type: "orchestration", Name: "Orchestration (Heat)", NewFunc: openstack.NewOrchestrationV1},
	{Type: "key-manager", Name: "Key Manager (Barbican)", NewFunc: openstack.NewKeyManagerV1},
	{Type: "workflowv2", Name: "Workflow (Mistral)", NewFunc: openstack.NewWorkflowV2},
}

// FetchServiceCatalog probes known OpenStack service types and returns
// available services with their endpoints.
func FetchServiceCatalog(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) []ServiceEntry {
	var entries []ServiceEntry
	for _, ks := range knownServices {
		entry := ServiceEntry{
			Name:      ks.Name,
			Type:      ks.Type,
			Available: false,
		}
		sc, err := ks.NewFunc(pc, eo)
		if err == nil && sc != nil {
			entry.Available = true

			var eps []EndpointInfo
			for _, avail := range []gophercloud.Availability{
				gophercloud.AvailabilityPublic,
				gophercloud.AvailabilityInternal,
				gophercloud.AvailabilityAdmin,
			} {
				eo2 := eo
				eo2.Availability = avail
				if ep, err := ks.NewFunc(pc, eo2); err == nil && ep != nil {
					eps = append(eps, EndpointInfo{
						Interface: string(avail),
						URL:       ep.Endpoint,
						Region:    eo2.Region,
					})
				}
			}
			entry.Endpoints = eps
		}
		entries = append(entries, entry)
	}
	return entries
}
