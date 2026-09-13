package network

import (
	"context"
	"encoding/json"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/testutil"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// Each mutation is exercised against a real HTTP server, including its error path.
func TestNetworkMutationsHTTP(t *testing.T) {
	ctx := context.Background()
	empty := ""
	no := false
	groups := []string{}
	pairs := []AddressPair{}
	routes := []HostRoute{}
	dns := []string{}
	for _, tc := range []struct {
		name, method, path, body, response string
		call                               func(*gophercloud.ServiceClient) error
	}{
		{"create network", "POST", "/networks", `{"network":{"name":"net","admin_state_up":false}}`, `{"network":{"id":"n","name":"net","status":"ACTIVE","shared":true}}`, func(c *gophercloud.ServiceClient) error {
			v, e := CreateNetwork(ctx, c, "net", false)
			if e == nil && (v.ID != "n" || v.Name != "net" || !v.Shared) {
				t.Errorf("network=%+v", v)
			}
			return e
		}},
		{"create router", "POST", "/routers", `{"router":{"name":"router","admin_state_up":false,"external_gateway_info":{"network_id":"external"}}}`, `{"router":{"id":"r","name":"router","external_gateway_info":{"network_id":"external"}}}`, func(c *gophercloud.ServiceClient) error {
			v, e := CreateRouter(ctx, c, "router", "external", false)
			if e == nil && (v.ID != "r" || v.ExternalGatewayNetworkID != "external") {
				t.Errorf("router=%+v", v)
			}
			return e
		}},
		{"create port", "POST", "/ports", `{"port":{"network_id":"n","fixed_ips":[{"subnet_id":"sub","ip_address":"10.0.0.5"}]}}`, `{"port":{"id":"p","network_id":"n","fixed_ips":[{"subnet_id":"sub","ip_address":"10.0.0.5"}]}}`, func(c *gophercloud.ServiceClient) error {
			v, e := CreatePort(ctx, c, "n", "sub", "10.0.0.5")
			if e == nil && (v.ID != "p" || len(v.FixedIPs) != 1 || v.FixedIPs[0].IPAddress != "10.0.0.5") {
				t.Errorf("port=%+v", v)
			}
			return e
		}},
		{"full port", "POST", "/ports", `{"port":{"network_id":"n","name":"port","admin_state_up":false,"security_groups":[],"port_security_enabled":false}}`, `{"port":{"id":"p","network_id":"n","port_security_enabled":false}}`, func(c *gophercloud.ServiceClient) error {
			v, e := CreatePortFull(ctx, c, PortCreateOpts{NetworkID: "n", Name: "port", SecurityGroups: groups, PortSecurityEnabled: &no})
			if e == nil && v.ID != "p" {
				t.Errorf("port=%+v", v)
			}
			return e
		}},
		{"update port clear", "PUT", "/ports/p", `{"port":{"name":"","description":"","admin_state_up":false,"security_groups":[],"allowed_address_pairs":[],"port_security_enabled":false}}`, `{"port":{"id":"p"}}`, func(c *gophercloud.ServiceClient) error {
			return UpdatePort(ctx, c, "p", PortUpdateOpts{Name: &empty, Description: &empty, AdminStateUp: &no, SecurityGroups: &groups, AllowedAddressPairs: &pairs, PortSecurityEnabled: &no})
		}},
		{"update subnet clear", "PUT", "/subnets/sub", `{"subnet":{"name":"","enable_dhcp":false,"gateway_ip":null,"dns_nameservers":[],"host_routes":[]}}`, `{"subnet":{"id":"sub"}}`, func(c *gophercloud.ServiceClient) error {
			return UpdateSubnet(ctx, c, "sub", SubnetUpdateOpts{Name: &empty, EnableDHCP: &no, GatewayIP: &empty, DNSNameservers: &dns, HostRoutes: &routes})
		}},
		{"create security group", "POST", "/security-groups", `{"security_group":{"name":"sg","description":"desc"}}`, `{"security_group":{"id":"sg","name":"sg","description":"desc"}}`, func(c *gophercloud.ServiceClient) error {
			v, e := CreateSecurityGroup(ctx, c, "sg", "desc")
			if e == nil && (v.ID != "sg" || v.Description != "desc") {
				t.Errorf("group=%+v", v)
			}
			return e
		}},
		{"update security group", "PUT", "/security-groups/sg", `{"security_group":{"name":"renamed","description":""}}`, `{"security_group":{"id":"sg","name":"renamed"}}`, func(c *gophercloud.ServiceClient) error {
			v, e := UpdateSecurityGroup(ctx, c, "sg", "renamed", &empty)
			if e == nil && (v.ID != "sg" || v.Name != "renamed") {
				t.Errorf("group=%+v", v)
			}
			return e
		}},
		{"add router subnet", "PUT", "/routers/r/add_router_interface", `{"subnet_id":"sub"}`, `{"id":"r","subnet_id":"sub"}`, func(c *gophercloud.ServiceClient) error { return AddRouterInterface(ctx, c, "r", "sub") }},
		{"add router port", "PUT", "/routers/r/add_router_interface", `{"port_id":"p"}`, `{"id":"r","port_id":"p"}`, func(c *gophercloud.ServiceClient) error { return AddRouterInterfaceByPort(ctx, c, "r", "p") }},
		{"remove router subnet", "PUT", "/routers/r/remove_router_interface", `{"subnet_id":"sub"}`, `{"id":"r","subnet_id":"sub"}`, func(c *gophercloud.ServiceClient) error { return RemoveRouterInterface(ctx, c, "r", "sub") }},
		{"associate floating ip", "PUT", "/floatingips/f", `{"floatingip":{"port_id":"p"}}`, `{"floatingip":{"id":"f","port_id":"p"}}`, func(c *gophercloud.ServiceClient) error { return AssociateFloatingIP(ctx, c, "f", "p") }},
		{"disassociate floating ip", "PUT", "/floatingips/f", `{"floatingip":{"port_id":null}}`, `{"floatingip":{"id":"f"}}`, func(c *gophercloud.ServiceClient) error { return DisassociateFloatingIP(ctx, c, "f") }},
		{"delete network", "DELETE", "/networks/n", "", "", func(c *gophercloud.ServiceClient) error { return DeleteNetwork(ctx, c, "n") }},
		{"delete subnet", "DELETE", "/subnets/sub", "", "", func(c *gophercloud.ServiceClient) error { return DeleteSubnet(ctx, c, "sub") }},
		{"delete router", "DELETE", "/routers/r", "", "", func(c *gophercloud.ServiceClient) error { return DeleteRouter(ctx, c, "r") }},
		{"delete port", "DELETE", "/ports/p", "", "", func(c *gophercloud.ServiceClient) error { return DeletePort(ctx, c, "p") }},
		{"delete group", "DELETE", "/security-groups/sg", "", "", func(c *gophercloud.ServiceClient) error { return DeleteSecurityGroup(ctx, c, "sg") }},
		{"delete rule", "DELETE", "/security-group-rules/rule", "", "", func(c *gophercloud.ServiceClient) error { return DeleteSecurityGroupRule(ctx, c, "rule") }},
		{"release floating ip", "DELETE", "/floatingips/f", "", "", func(c *gophercloud.ServiceClient) error { return ReleaseFloatingIP(ctx, c, "f") }},
	} {
		for _, fail := range []bool{false, true} {
			name := tc.name
			if fail {
				name += "/failure"
			}
			t.Run(name, func(t *testing.T) {
				calls := 0
				client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Method != tc.method || r.URL.Path != tc.path {
						t.Errorf("request=%s %s want=%s %s", r.Method, r.URL.Path, tc.method, tc.path)
					}
					if tc.body != "" {
						var got, want any
						if e := json.NewDecoder(r.Body).Decode(&got); e != nil {
							t.Error(e)
						}
						if e := json.Unmarshal([]byte(tc.body), &want); e != nil {
							t.Error(e)
						}
						if !reflect.DeepEqual(got, want) {
							t.Errorf("body=%v want=%v", got, want)
						}
					}
					w.Header().Set("Content-Type", "application/json")
					if fail {
						w.WriteHeader(409)
						w.Write([]byte(`{"error":"conflict"}`))
						return
					}
					status := 200
					if tc.method == "POST" {
						status = 201
					}
					if tc.method == "DELETE" {
						status = 204
					}
					w.WriteHeader(status)
					w.Write([]byte(tc.response))
				}))
				defer close()
				err := tc.call(client)
				if calls != 1 || (err != nil) != fail {
					t.Fatalf("calls=%d err=%v", calls, err)
				}
				if fail && !gophercloud.ResponseCodeIs(err, 409) {
					t.Fatalf("lost HTTP cause: %v", err)
				}
			})
		}
	}
}

func TestCloneSecurityGroupPartialFailureHTTP(t *testing.T) {
	for _, stage := range []string{"source", "create", "second rule", "success"} {
		t.Run(stage, func(t *testing.T) {
			var calls []string
			ruleCalls := 0
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.Method+" "+r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /security-groups/src":
					if stage == "source" {
						w.WriteHeader(404)
						return
					}
					w.Write([]byte(`{"security_group":{"id":"src","security_group_rules":[{"direction":"egress","ethertype":"IPv4"},{"direction":"ingress","ethertype":"IPv4","protocol":"tcp","port_range_min":22,"port_range_max":22,"remote_ip_prefix":"10.0.0.0/24"},{"direction":"ingress","ethertype":"IPv6","protocol":"udp","port_range_min":53,"port_range_max":53,"remote_group_id":"remote"}]}}`))
				case "POST /security-groups":
					var body map[string]map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if !reflect.DeepEqual(body["security_group"], map[string]any{"name": "copy", "description": "new desc"}) {
						t.Errorf("body=%v", body)
					}
					if stage == "create" {
						w.WriteHeader(403)
						return
					}
					w.WriteHeader(201)
					w.Write([]byte(`{"security_group":{"id":"dst","name":"copy"}}`))
				case "POST /security-group-rules":
					ruleCalls++
					var body map[string]map[string]any
					if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
						t.Error(e)
					}
					rule := body["security_group_rule"]
					want := map[string]any{"security_group_id": "dst", "direction": "ingress", "ethertype": "IPv4", "protocol": "tcp", "port_range_min": float64(22), "port_range_max": float64(22), "remote_ip_prefix": "10.0.0.0/24"}
					if ruleCalls == 2 {
						want = map[string]any{"security_group_id": "dst", "direction": "ingress", "ethertype": "IPv6", "protocol": "udp", "port_range_min": float64(53), "port_range_max": float64(53), "remote_group_id": "remote"}
					}
					if !reflect.DeepEqual(rule, want) {
						t.Errorf("rule=%v want=%v", rule, want)
					}
					if stage == "second rule" && ruleCalls == 2 {
						w.WriteHeader(409)
						return
					}
					w.WriteHeader(201)
					w.Write([]byte(`{"security_group_rule":{"id":"created"}}`))
				case "GET /security-groups/dst":
					w.Write([]byte(`{"security_group":{"id":"dst","name":"copy","description":"new desc","security_group_rules":[{"id":"created","direction":"ingress","ethertype":"IPv4","protocol":"tcp","port_range_min":22,"port_range_max":22}]}}`))
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer close()
			got, err := CloneSecurityGroup(context.Background(), client, "src", "copy", "new desc")
			want := []string{"GET /security-groups/src"}
			if stage != "source" {
				want = append(want, "POST /security-groups")
			}
			if stage == "second rule" || stage == "success" {
				want = append(want, "POST /security-group-rules", "POST /security-group-rules")
			}
			if stage == "success" {
				want = append(want, "GET /security-groups/dst")
			}
			if !reflect.DeepEqual(calls, want) {
				t.Errorf("calls=%v want=%v", calls, want)
			}
			if stage == "success" {
				if err != nil || got == nil || got.ID != "dst" || len(got.Rules) != 1 || got.Rules[0].PortRangeMin != 22 {
					t.Fatalf("got=%+v err=%v", got, err)
				}
			} else {
				if err == nil || got != nil {
					t.Fatalf("got=%v err=%v", got, err)
				}
				if stage == "second rule" && !strings.Contains(err.Error(), "cloning rule") {
					t.Fatal(err)
				}
			}
			// Current contract stops on a failed rule without deleting the partially created group.
		})
	}
}
