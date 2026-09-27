package network

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

// filteringPortsServer is a fake Neutron /ports endpoint that honours the
// device_id, device_owner (repeatable) and network_id filters and paginates
// with a page size of two via ports_links, like Neutron does.
func filteringPortsServer(t *testing.T, allPorts []map[string]any) *gophercloud.ServiceClient {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/ports") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		var matched []map[string]any
		for _, p := range allPorts {
			if v := q.Get("device_id"); v != "" && p["device_id"] != v {
				continue
			}
			if v := q.Get("network_id"); v != "" && p["network_id"] != v {
				continue
			}
			if owners := q["device_owner"]; len(owners) > 0 {
				ok := false
				for _, o := range owners {
					if p["device_owner"] == o {
						ok = true
					}
				}
				if !ok {
					continue
				}
			}
			matched = append(matched, p)
		}
		start := 0
		if marker := q.Get("marker"); marker != "" {
			for i, p := range matched {
				if p["id"] == marker {
					start = i + 1
				}
			}
		}
		end := min(start+2, len(matched))
		body := map[string]any{"ports": matched[start:end]}
		if end < len(matched) {
			nq := r.URL.Query()
			nq.Set("marker", matched[end-1]["id"].(string))
			body["ports_links"] = []map[string]string{{"rel": "next", "href": srv.URL + r.URL.Path + "?" + nq.Encode()}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{HTTPClient: *srv.Client()},
		Endpoint:       srv.URL + "/",
	}
}

func fakePort(id, deviceID, owner, networkID, subnetID, ip string) map[string]any {
	return map[string]any{
		"id":           id,
		"device_id":    deviceID,
		"device_owner": owner,
		"network_id":   networkID,
		"fixed_ips":    []map[string]string{{"subnet_id": subnetID, "ip_address": ip}},
	}
}

func haDVRRouterPorts() []map[string]any {
	return []map[string]any{
		fakePort("p-legacy", "r1", "network:router_interface", "net-a", "sub-a", "10.0.0.1"),
		fakePort("p-gw", "r1", "network:router_gateway", "ext", "sub-ext", "203.0.113.5"),
		fakePort("p-ha-aux", "r1", "network:router_ha_interface", "ha-net", "sub-ha", "169.254.192.1"),
		fakePort("p-ha", "r1", "network:ha_router_replicated_interface", "net-b", "sub-b", "10.1.0.1"),
		fakePort("p-snat", "r1", "network:router_centralized_snat", "net-c", "sub-c", "10.2.0.9"),
		fakePort("p-dvr", "r1", "network:router_interface_distributed", "net-c", "sub-c", "10.2.0.1"),
		fakePort("p-other", "r2", "network:router_interface", "net-a", "sub-a", "10.0.0.2"),
		fakePort("p-vm", "vm1", "compute:nova", "net-a", "sub-a", "10.0.0.50"),
	}
}

func TestListRouterInterfacesRecognizesHAAndDVR(t *testing.T) {
	client := filteringPortsServer(t, haDVRRouterPorts())
	ifaces, err := ListRouterInterfaces(context.Background(), client, "r1")
	if err != nil {
		t.Fatalf("ListRouterInterfaces: %v", err)
	}
	got := map[string]string{}
	for _, i := range ifaces {
		got[i.PortID] = i.SubnetID
	}
	want := map[string]string{"p-legacy": "sub-a", "p-ha": "sub-b", "p-dvr": "sub-c"}
	if len(got) != len(want) {
		t.Fatalf("interfaces = %v, want %v", got, want)
	}
	for port, sub := range want {
		if got[port] != sub {
			t.Errorf("interface %s: subnet %q, want %q (all: %v)", port, got[port], sub, got)
		}
	}
}

func TestFindRouterPortOnNetworkRecognizesHAAndDVR(t *testing.T) {
	client := filteringPortsServer(t, haDVRRouterPorts())
	for network, want := range map[string]string{"net-a": "p-legacy", "net-b": "p-ha", "net-c": "p-dvr", "ha-net": "", "ext": ""} {
		p, err := FindRouterPortOnNetwork(context.Background(), client, "r1", network)
		if err != nil {
			t.Fatalf("FindRouterPortOnNetwork(%s): %v", network, err)
		}
		got := ""
		if p != nil {
			got = p.ID
		}
		if got != want {
			t.Errorf("FindRouterPortOnNetwork(%s) = %q, want %q", network, got, want)
		}
	}
}
