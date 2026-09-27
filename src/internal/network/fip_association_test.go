package network

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fipUpdateRecorder records PUT bodies sent to /floatingips/{id}.
func fipUpdateRecorder(t *testing.T, bodies *[]string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "floatingips/fip-1") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		*bodies = append(*bodies, string(raw))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"floatingip":{"id":"fip-1"}}`))
	}
}

func TestAssociateFloatingIPRejectsEmptyPort(t *testing.T) {
	var bodies []string
	client := fakeNeutronClient(t, fipUpdateRecorder(t, &bodies))
	for _, port := range []string{"", "   "} {
		if err := AssociateFloatingIP(context.Background(), client, "fip-1", port); err == nil {
			t.Errorf("AssociateFloatingIP(%q) succeeded, want error", port)
		}
	}
	if len(bodies) != 0 {
		t.Fatalf("empty port association reached the API: %q", bodies)
	}
}

func TestAssociateFloatingIPKeepsValidPortAndDisassociateSendsNull(t *testing.T) {
	var bodies []string
	client := fakeNeutronClient(t, fipUpdateRecorder(t, &bodies))
	if err := AssociateFloatingIP(context.Background(), client, "fip-1", "port-9"); err != nil {
		t.Fatalf("AssociateFloatingIP: %v", err)
	}
	if err := DisassociateFloatingIP(context.Background(), client, "fip-1"); err != nil {
		t.Fatalf("DisassociateFloatingIP: %v", err)
	}
	if len(bodies) != 2 || !strings.Contains(bodies[0], `"port_id":"port-9"`) || !strings.Contains(bodies[1], `"port_id":null`) {
		t.Fatalf("bodies = %q", bodies)
	}
}

func TestListFloatingIPTargetsAnnotatesRoutedIPv4Addresses(t *testing.T) {
	allPorts := []map[string]any{
		// Server ports: net-a is routed to ext-1, net-b to ext-2, net-c is
		// isolated; net-a also carries an IPv6 address (not NAT-able).
		{"id": "port-b", "name": "backend", "device_id": "srv", "network_id": "net-b",
			"fixed_ips": []map[string]string{{"subnet_id": "sub-b", "ip_address": "10.1.0.5"}}},
		{"id": "port-a", "name": "frontend", "device_id": "srv", "network_id": "net-a",
			"fixed_ips": []map[string]string{{"subnet_id": "sub-a6", "ip_address": "2001:db8::5"}, {"subnet_id": "sub-a", "ip_address": "10.0.0.5"}}},
		{"id": "port-c", "name": "isolated", "device_id": "srv", "network_id": "net-c",
			"fixed_ips": []map[string]string{{"subnet_id": "sub-c", "ip_address": "10.2.0.5"}}},
		fakePort("r1-a", "r1", "network:router_interface", "net-a", "sub-a", "10.0.0.1"),
		fakePort("r2-b", "r2", "network:ha_router_replicated_interface", "net-b", "sub-b", "10.1.0.1"),
		fakePort("r3-c", "r3", "network:router_interface", "net-c", "sub-c", "10.2.0.1"),
	}
	portsClient := filteringPortsServer(t, allPorts)
	routersJSON := `{"routers":[
		{"id":"r1","external_gateway_info":{"network_id":"ext-1"}},
		{"id":"r2","external_gateway_info":{"network_id":"ext-2"}},
		{"id":"r3"}]}`
	client := fakeNeutronClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/routers") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(routersJSON))
			return
		}
		// Proxy port queries to the filtering fake.
		resp, err := portsClient.HTTPClient.Get(portsClient.Endpoint + strings.TrimPrefix(r.URL.Path, "/") + "?" + r.URL.RawQuery)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))

	targets, err := ListFloatingIPTargets(context.Background(), client, "srv")
	if err != nil {
		t.Fatalf("ListFloatingIPTargets: %v", err)
	}
	var got []string
	for _, tg := range targets {
		got = append(got, tg.PortID+"/"+tg.IPAddress+"->"+strings.Join(tg.ExternalNetworkIDs, "+"))
	}
	want := "port-b/10.1.0.5->ext-2,port-a/10.0.0.5->ext-1,port-c/10.2.0.5->"
	if strings.Join(got, ",") != want {
		t.Fatalf("targets = %v, want %s", got, want)
	}
	if !targets[1].ReachableFrom("ext-1") || targets[1].ReachableFrom("ext-2") || targets[2].ReachableFrom("ext-1") {
		t.Errorf("ReachableFrom mismatch: %+v", targets)
	}
	if targets[1].Label() != "10.0.0.5 (port frontend)" {
		t.Errorf("Label = %q", targets[1].Label())
	}
}

func TestAssociateFloatingIPToAddressSendsChosenPortAndAddress(t *testing.T) {
	var bodies []string
	client := fakeNeutronClient(t, fipUpdateRecorder(t, &bodies))
	if err := AssociateFloatingIPToAddress(context.Background(), client, "fip-1", "port-a", "10.0.0.5"); err != nil {
		t.Fatalf("AssociateFloatingIPToAddress: %v", err)
	}
	if len(bodies) != 1 || !strings.Contains(bodies[0], `"port_id":"port-a"`) || !strings.Contains(bodies[0], `"fixed_ip_address":"10.0.0.5"`) {
		t.Fatalf("bodies = %q", bodies)
	}
}
