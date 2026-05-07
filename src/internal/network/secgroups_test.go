package network

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// secGroupsFixture is a minimal neutron API paginated security group list
// response with nested security_group_rules.
const secGroupsFixture = `{
  "security_groups": [
    {
      "id": "sg-default-001",
      "name": "default",
      "description": "Default security group",
      "security_group_rules": [
        {
          "id": "rule-egress-all",
          "direction": "egress",
          "ethertype": "IPv6",
          "protocol": "",
          "port_range_min": 0,
          "port_range_max": 0,
          "remote_ip_prefix": "::/0"
        },
        {
          "id": "rule-egress-all-v4",
          "direction": "egress",
          "ethertype": "IPv4",
          "protocol": "",
          "port_range_min": 0,
          "port_range_max": 0,
          "remote_ip_prefix": "0.0.0.0/0"
        }
      ]
    },
    {
      "id": "sg-web-002",
      "name": "web-servers",
      "description": "HTTP/HTTPS access",
      "security_group_rules": [
        {
          "id": "rule-ingress-ssh",
          "direction": "ingress",
          "ethertype": "IPv4",
          "protocol": "tcp",
          "port_range_min": 22,
          "port_range_max": 22,
          "remote_ip_prefix": "10.0.0.0/8"
        },
        {
          "id": "rule-ingress-http",
          "direction": "ingress",
          "ethertype": "IPv4",
          "protocol": "tcp",
          "port_range_min": 80,
          "port_range_max": 80,
          "remote_ip_prefix": "0.0.0.0/0"
        },
        {
          "id": "rule-ingress-https",
          "direction": "ingress",
          "ethertype": "IPv4",
          "protocol": "tcp",
          "port_range_min": 443,
          "port_range_max": 443,
          "remote_ip_prefix": "0.0.0.0/0"
        },
        {
          "id": "rule-egress-all",
          "direction": "egress",
          "ethertype": "IPv4",
          "protocol": "",
          "port_range_min": 0,
          "port_range_max": 0,
          "remote_ip_prefix": "0.0.0.0/0"
        }
      ]
    }
  ]
}`

func TestListSecurityGroups(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "security-groups") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(secGroupsFixture))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client := fakeNeutronClient(handler)
	ctx := context.Background()

	sgs, err := ListSecurityGroups(ctx, client)
	if err != nil {
		t.Fatalf("ListSecurityGroups() error: %v", err)
	}
	if len(sgs) != 2 {
		t.Fatalf("expected 2 security groups, got %d", len(sgs))
	}

	// Group 1: default, 2 egress rules (IPv4 + IPv6)
	sg1 := sgs[0]
	if sg1.ID != "sg-default-001" {
		t.Errorf("unexpected ID: %s", sg1.ID)
	}
	if sg1.Name != "default" {
		t.Errorf("unexpected Name: %s", sg1.Name)
	}
	if sg1.Description != "Default security group" {
		t.Errorf("unexpected Description: %s", sg1.Description)
	}
	if len(sg1.Rules) != 2 {
		t.Fatalf("expected 2 rules in default SG, got %d", len(sg1.Rules))
	}

	// Verify nested rule mapping: egress IPv6
	r1 := sg1.Rules[0]
	if r1.ID != "rule-egress-all" {
		t.Errorf("unexpected rule ID: %s", r1.ID)
	}
	if r1.Direction != "egress" {
		t.Errorf("unexpected Direction: %s", r1.Direction)
	}
	if r1.EtherType != "IPv6" {
		t.Errorf("unexpected EtherType: %s", r1.EtherType)
	}
	if r1.PortRangeMin != 0 {
		t.Errorf("unexpected PortRangeMin: %d", r1.PortRangeMin)
	}
	if r1.PortRangeMax != 0 {
		t.Errorf("unexpected PortRangeMax: %d", r1.PortRangeMax)
	}
	if r1.RemoteIPPrefix != "::/0" {
		t.Errorf("unexpected RemoteIPPrefix: %s", r1.RemoteIPPrefix)
	}

	// Verify nested rule mapping: egress IPv4
	r2 := sg1.Rules[1]
	if r2.EtherType != "IPv4" {
		t.Errorf("unexpected EtherType: %s", r2.EtherType)
	}
	if r2.RemoteIPPrefix != "0.0.0.0/0" {
		t.Errorf("unexpected RemoteIPPrefix: %s", r2.RemoteIPPrefix)
	}

	// Group 2: web-servers, 3 ingress rules + 1 egress
	sg2 := sgs[1]
	if sg2.ID != "sg-web-002" {
		t.Errorf("unexpected ID: %s", sg2.ID)
	}
	if sg2.Name != "web-servers" {
		t.Errorf("unexpected Name: %s", sg2.Name)
	}
	if sg2.Description != "HTTP/HTTPS access" {
		t.Errorf("unexpected Description: %s", sg2.Description)
	}
	if len(sg2.Rules) != 4 {
		t.Fatalf("expected 4 rules in web-servers SG, got %d", len(sg2.Rules))
	}

	// SSH ingress rule
	sshRule := sg2.Rules[0]
	if sshRule.Direction != "ingress" {
		t.Errorf("unexpected Direction: %s", sshRule.Direction)
	}
	if sshRule.Protocol != "tcp" {
		t.Errorf("unexpected Protocol: %s", sshRule.Protocol)
	}
	if sshRule.PortRangeMin != 22 || sshRule.PortRangeMax != 22 {
		t.Errorf("unexpected port range: %d/%d", sshRule.PortRangeMin, sshRule.PortRangeMax)
	}
	if sshRule.RemoteIPPrefix != "10.0.0.0/8" {
		t.Errorf("unexpected RemoteIPPrefix: %s", sshRule.RemoteIPPrefix)
	}

	// HTTP ingress rule
	httpRule := sg2.Rules[1]
	if httpRule.Direction != "ingress" {
		t.Errorf("unexpected Direction: %s", httpRule.Direction)
	}
	if httpRule.PortRangeMin != 80 || httpRule.PortRangeMax != 80 {
		t.Errorf("unexpected HTTP port range: %d/%d", httpRule.PortRangeMin, httpRule.PortRangeMax)
	}

	// HTTPS ingress rule
	httpsRule := sg2.Rules[2]
	if httpsRule.PortRangeMin != 443 || httpsRule.PortRangeMax != 443 {
		t.Errorf("unexpected HTTPS port range: %d/%d", httpsRule.PortRangeMin, httpsRule.PortRangeMax)
	}

	// Egress rule
	egressRule := sg2.Rules[3]
	if egressRule.Direction != "egress" {
		t.Errorf("unexpected Direction: %s", egressRule.Direction)
	}
	if egressRule.EtherType != "IPv4" {
		t.Errorf("unexpected EtherType: %s", egressRule.EtherType)
	}
}
