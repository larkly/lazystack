package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

// createSubnetFixture is a minimal neutron subnet create response.
const createSubnetFixture = `{
  "subnet": {
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "name": "test-subnet",
    "network_id": "3d7e11e8-8f4c-4b78-bd91-06cdab3aab91",
    "cidr": "192.168.1.0/24",
    "gateway_ip": "192.168.1.1",
    "ip_version": 4,
    "enable_dhcp": true,
    "allocation_pools": [
      {"start": "192.168.1.100", "end": "192.168.1.200"}
    ]
  }
}`

// updateSubnetFixture is a minimal neutron subnet update response.
const updateSubnetFixture = `{
  "subnet": {
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "name": "renamed-subnet",
    "network_id": "3d7e11e8-8f4c-4b78-bd91-06cdab3aab91",
    "cidr": "192.168.1.0/24",
    "gateway_ip": "192.168.1.1",
    "ip_version": 4,
    "enable_dhcp": true,
    "dns_nameservers": ["1.1.1.1", "9.9.9.9"]
  }
}`

// subnetPoolsFixture is a minimal neutron subnet pool list response.
const subnetPoolsFixture = `{
  "subnetpools": [
    {
      "id": "sp-001",
      "name": "default-pool",
      "prefixes": ["10.0.0.0/8"],
      "ip_version": 4,
      "default_prefixlen": 24,
      "min_prefixlen": 8,
      "max_prefixlen": 32
    },
    {
      "id": "sp-002",
      "name": "ipv6-pool",
      "prefixes": ["fd00::/8"],
      "ip_version": 6,
      "default_prefixlen": 64,
      "min_prefixlen": 64,
      "max_prefixlen": 128
    }
  ]
}`

func fakeNeutronClientSubnets(handler http.Handler) *gophercloud.ServiceClient {
	srv := httptest.NewServer(handler)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			HTTPClient: *srv.Client(),
		},
		Endpoint: srv.URL + "/",
	}
}

func TestCreateSubnet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "subnets") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(createSubnetFixture))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	client := fakeNeutronClientSubnets(handler)
	ctx := context.Background()

	subnet, err := CreateSubnet(ctx, client, SubnetCreateOpts{
		NetworkID:  "3d7e11e8-8f4c-4b78-bd91-06cdab3aab91",
		Name:       "test-subnet",
		CIDR:       "192.168.1.0/24",
		IPVersion:  4,
		GatewayIP:  "192.168.1.1",
		EnableDHCP: true,
	})
	if err != nil {
		t.Fatalf("CreateSubnet() error: %v", err)
	}

	if subnet.ID != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("unexpected ID: %s", subnet.ID)
	}
	if subnet.Name != "test-subnet" {
		t.Errorf("unexpected Name: %s", subnet.Name)
	}
	if subnet.NetworkID != "3d7e11e8-8f4c-4b78-bd91-06cdab3aab91" {
		t.Errorf("unexpected NetworkID: %s", subnet.NetworkID)
	}
	if subnet.CIDR != "192.168.1.0/24" {
		t.Errorf("unexpected CIDR: %s", subnet.CIDR)
	}
	if subnet.GatewayIP != "192.168.1.1" {
		t.Errorf("unexpected GatewayIP: %s", subnet.GatewayIP)
	}
	if subnet.IPVersion != 4 {
		t.Errorf("unexpected IPVersion: %d", subnet.IPVersion)
	}
	if !subnet.EnableDHCP {
		t.Errorf("expected EnableDHCP=true")
	}
	if len(subnet.AllocationPools) != 1 {
		t.Fatalf("expected 1 AllocationPool, got %d", len(subnet.AllocationPools))
	}
	if subnet.AllocationPools[0].Start != "192.168.1.100" {
		t.Errorf("unexpected pool Start: %s", subnet.AllocationPools[0].Start)
	}
	if subnet.AllocationPools[0].End != "192.168.1.200" {
		t.Errorf("unexpected pool End: %s", subnet.AllocationPools[0].End)
	}
}

func TestUpdateSubnet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "subnets") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(updateSubnetFixture))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	client := fakeNeutronClientSubnets(handler)
	ctx := context.Background()

	newName := "renamed-subnet"
	dnsServers := []string{"1.1.1.1", "9.9.9.9"}

	err := UpdateSubnet(ctx, client, "a1b2c3d4-e5f6-7890-abcd-ef1234567890", SubnetUpdateOpts{
		Name:           &newName,
		DNSNameservers: &dnsServers,
	})
	if err != nil {
		t.Fatalf("UpdateSubnet() error: %v", err)
	}
}

func TestDeleteSubnet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "subnets") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	client := fakeNeutronClientSubnets(handler)
	ctx := context.Background()

	err := DeleteSubnet(ctx, client, "a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	if err != nil {
		t.Fatalf("DeleteSubnet() error: %v", err)
	}
}

func TestListSubnetPools(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "subnetpools") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(subnetPoolsFixture))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	client := fakeNeutronClientSubnets(handler)
	ctx := context.Background()

	pools, err := ListSubnetPools(ctx, client)
	if err != nil {
		t.Fatalf("ListSubnetPools() error: %v", err)
	}
	if len(pools) != 2 {
		t.Fatalf("expected 2 subnet pools, got %d", len(pools))
	}

	p1 := pools[0]
	if p1.ID != "sp-001" {
		t.Errorf("unexpected ID: %s", p1.ID)
	}
	if p1.Name != "default-pool" {
		t.Errorf("unexpected Name: %s", p1.Name)
	}
	if len(p1.Prefixes) != 1 {
		t.Errorf("expected 1 prefix, got %d", len(p1.Prefixes))
	}
	if p1.Prefixes[0] != "10.0.0.0/8" {
		t.Errorf("unexpected prefix: %s", p1.Prefixes[0])
	}
	if p1.DefaultPrefixLen != 24 {
		t.Errorf("unexpected DefaultPrefixLen: %d", p1.DefaultPrefixLen)
	}

	p2 := pools[1]
	if p2.ID != "sp-002" {
		t.Errorf("unexpected ID: %s", p2.ID)
	}
	if p2.Name != "ipv6-pool" {
		t.Errorf("unexpected Name: %s", p2.Name)
	}
	if p2.DefaultPrefixLen != 64 {
		t.Errorf("unexpected DefaultPrefixLen for IPv6 pool: %d", p2.DefaultPrefixLen)
	}
}
