package compute

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/hypervisors"
	"github.com/gophercloud/gophercloud/v2/pagination"
)

// Hypervisor is a simplified representation of an OpenStack hypervisor.
type Hypervisor struct {
	ID               string
	Name             string // HypervisorHostname
	Type             string // HypervisorType
	Status           string
	State            string
	VCPUs            int
	VCPUsUsed        int
	MemoryMB         int
	MemoryMBUsed     int
	LocalGB          int
	LocalGBUsed      int
	RunningVMs       int
	HostIP           string
}

// ListHypervisors returns all hypervisors visible to the user.
func ListHypervisors(ctx context.Context, client *gophercloud.ServiceClient) ([]Hypervisor, error) {
	pager := hypervisors.List(client, nil)
	var result []Hypervisor
	err := pager.EachPage(ctx, func(ctx context.Context, page pagination.Page) (bool, error) {
		list, err := hypervisors.ExtractHypervisors(page)
		if err != nil {
			return false, err
		}
		for _, h := range list {
			result = append(result, Hypervisor{
				ID:         h.ID,
				Name:       h.HypervisorHostname,
				Type:       h.HypervisorType,
				Status:     h.Status,
				State:      h.State,
				VCPUs:      h.VCPUs,
				VCPUsUsed:  h.VCPUsUsed,
				MemoryMB:   h.MemoryMB,
				MemoryMBUsed: h.MemoryMBUsed,
				LocalGB:    h.LocalGB,
				LocalGBUsed: h.LocalGBUsed,
				RunningVMs: h.RunningVMs,
				HostIP:     h.HostIP,
			})
		}
		return true, nil
	})
	return result, err
}
