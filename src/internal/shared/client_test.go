package shared

import (
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

func TestRequireClient(t *testing.T) {
	err := RequireClient(nil, ServiceBlockStorage)
	if err == nil || err.Error() != "block storage (Cinder) service is not available in this cloud" {
		t.Errorf("nil client: err=%v", err)
	}
	if err := RequireClient(&gophercloud.ServiceClient{}, ServiceCompute); err != nil {
		t.Errorf("non-nil client: err=%v", err)
	}
}
