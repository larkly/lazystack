package shared

import (
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
)

// Service names for RequireClient, as shown to the user.
const (
	ServiceCompute      = "compute (Nova)"
	ServiceNetwork      = "networking (Neutron)"
	ServiceBlockStorage = "block storage (Cinder)"
	ServiceImage        = "image (Glance)"
	ServiceLoadBalancer = "load balancer (Octavia)"
)

// RequireClient guards against a nil service client on clouds lacking the
// service; gophercloud methods panic on nil *ServiceClient receivers.
func RequireClient(sc *gophercloud.ServiceClient, service string) error {
	if sc == nil {
		Debugf("[shared] RequireClient: no %s client", service)
		return fmt.Errorf("%s service is not available in this cloud", service)
	}
	return nil
}
