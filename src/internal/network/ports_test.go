package network

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// portsFixture is a minimal neutron API paginated port list response.
const portsFixture = `{
  "ports": [
    {
      "id": "p1-port-id-001",
      "name": "web-port",
      "description": "Port for web server",
      "status": "ACTIVE",
      "mac_address": "fa:16:3e:12:34:56",
      "fixed_ips": [
        {
          "subnet_id": "subnet-a1b2",
          "ip_address": "10.0.0.10"
        }
      ],
      "device_owner": "compute:nova",
      "device_id": "server-uuid-001",
      "network_id": "net-uuid-001",
      "security_groups": ["sg-abc123", "sg-def456"],
      "allowed_address_pairs": [
        {
          "ip_address": "10.0.0.20",
          "mac_address": "fa:16:3e:aa:bb:cc"
        }
      ],
      "admin_state_up": true,
      "port_security_enabled": true,
      "tenant_id": "project-uuid-001"
    },
    {
      "id": "p2-port-id-002",
      "name": "db-port",
      "status": "DOWN",
      "mac_address": "fa:16:3e:78:90:ab",
      "fixed_ips": [
        {
          "subnet_id": "subnet-c3d4",
          "ip_address": "10.0.0.20"
        },
        {
          "subnet_id": "subnet-e5f6",
          "ip_address": "fd00::20"
        }
      ],
      "device_owner": "compute:nova",
      "device_id": "server-uuid-002",
      "network_id": "net-uuid-001",
      "security_groups": [],
      "allowed_address_pairs": [],
      "admin_state_up": false,
      "port_security_enabled": false,
      "tenant_id": "project-uuid-001"
    }
  ]
}`

func TestListPorts(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "ports") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(portsFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeNeutronClient(handler)
	ctx := context.Background()

	ports, err := ListPorts(ctx, client, "net-uuid-001")
	if err != nil {
		t.Fatalf("ListPorts() error: %v", err)
	}
	if len(ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(ports))
	}

	// Verify first port
	p1 := ports[0]
	if p1.ID != "p1-port-id-001" {
		t.Errorf("unexpected ID: %s", p1.ID)
	}
	if p1.Name != "web-port" {
		t.Errorf("unexpected Name: %s", p1.Name)
	}
	if p1.Description != "Port for web server" {
		t.Errorf("unexpected Description: %s", p1.Description)
	}
	if p1.Status != "ACTIVE" {
		t.Errorf("unexpected Status: %s", p1.Status)
	}
	if p1.MACAddress != "fa:16:3e:12:34:56" {
		t.Errorf("unexpected MACAddress: %s", p1.MACAddress)
	}
	if p1.DeviceOwner != "compute:nova" {
		t.Errorf("unexpected DeviceOwner: %s", p1.DeviceOwner)
	}
	if p1.DeviceID != "server-uuid-001" {
		t.Errorf("unexpected DeviceID: %s", p1.DeviceID)
	}
	if p1.NetworkID != "net-uuid-001" {
		t.Errorf("unexpected NetworkID: %s", p1.NetworkID)
	}
	if !p1.AdminStateUp {
		t.Error("expected AdminStateUp to be true")
	}
	if !p1.PortSecurityEnabled {
		t.Error("expected PortSecurityEnabled to be true")
	}
	if len(p1.FixedIPs) != 1 {
		t.Fatalf("expected 1 FixedIP, got %d", len(p1.FixedIPs))
	}
	if p1.FixedIPs[0].SubnetID != "subnet-a1b2" {
		t.Errorf("unexpected FixedIP SubnetID: %s", p1.FixedIPs[0].SubnetID)
	}
	if p1.FixedIPs[0].IPAddress != "10.0.0.10" {
		t.Errorf("unexpected FixedIP IPAddress: %s", p1.FixedIPs[0].IPAddress)
	}
	if len(p1.SecurityGroups) != 2 {
		t.Fatalf("expected 2 SecurityGroups, got %d", len(p1.SecurityGroups))
	}
	if p1.SecurityGroups[0] != "sg-abc123" {
		t.Errorf("unexpected SecurityGroup[0]: %s", p1.SecurityGroups[0])
	}
	if len(p1.AllowedAddressPairs) != 1 {
		t.Fatalf("expected 1 AllowedAddressPair, got %d", len(p1.AllowedAddressPairs))
	}
	if p1.AllowedAddressPairs[0].IPAddress != "10.0.0.20" {
		t.Errorf("unexpected address pair IP: %s", p1.AllowedAddressPairs[0].IPAddress)
	}

	// Verify second port (down, dual IP, no security groups)
	p2 := ports[1]
	if p2.ID != "p2-port-id-002" {
		t.Errorf("unexpected second ID: %s", p2.ID)
	}
	if p2.Name != "db-port" {
		t.Errorf("unexpected second Name: %s", p2.Name)
	}
	if p2.Status != "DOWN" {
		t.Errorf("unexpected second Status: %s", p2.Status)
	}
	if p2.AdminStateUp {
		t.Error("expected AdminStateUp to be false on second port")
	}
	if p2.PortSecurityEnabled {
		t.Error("expected PortSecurityEnabled to be false on second port")
	}
	if len(p2.FixedIPs) != 2 {
		t.Fatalf("expected 2 FixedIPs on second port, got %d", len(p2.FixedIPs))
	}
	if p2.FixedIPs[1].IPAddress != "fd00::20" {
		t.Errorf("unexpected second FixedIP: %s", p2.FixedIPs[1].IPAddress)
	}
	if len(p2.SecurityGroups) != 0 {
		t.Errorf("expected 0 SecurityGroups, got %d", len(p2.SecurityGroups))
	}
	if len(p2.AllowedAddressPairs) != 0 {
		t.Errorf("expected 0 AllowedAddressPairs, got %d", len(p2.AllowedAddressPairs))
	}
}
