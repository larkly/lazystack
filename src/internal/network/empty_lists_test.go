package network

import (
	"context"
	"io"
	"net/http"
	"testing"
)

// bodyRecorder answers any PUT with an empty resource and records the raw
// request body.
func bodyRecorder(resource string, bodies *[]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*bodies = append(*bodies, string(raw))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"` + resource + `":{"id":"x"}}`))
	}
}

func TestUpdatePortNilSecurityGroupsSendsEmptyArray(t *testing.T) {
	var bodies []string
	client := fakeNeutronClient(t, bodyRecorder("port", &bodies))
	var none []string
	if err := UpdatePort(context.Background(), client, "p", PortUpdateOpts{SecurityGroups: &none}); err != nil {
		t.Fatalf("UpdatePort: %v", err)
	}
	if err := UpdatePort(context.Background(), client, "p", PortUpdateOpts{}); err != nil {
		t.Fatalf("UpdatePort: %v", err)
	}
	if bodies[0] != `{"port":{"security_groups":[]}}` {
		t.Errorf("cleared body = %s", bodies[0])
	}
	if bodies[1] != `{"port":{}}` {
		t.Errorf("untouched body = %s", bodies[1])
	}
}

func TestUpdateSubnetEmptyListsSerialiseAsArrays(t *testing.T) {
	var bodies []string
	client := fakeNeutronClient(t, bodyRecorder("subnet", &bodies))
	var noDNS []string
	noPools := []AllocationPool{}
	if err := UpdateSubnet(context.Background(), client, "s", SubnetUpdateOpts{DNSNameservers: &noDNS, AllocationPools: &noPools}); err != nil {
		t.Fatalf("UpdateSubnet: %v", err)
	}
	pools := []AllocationPool{{Start: "10.0.0.2", End: "10.0.0.9"}}
	if err := UpdateSubnet(context.Background(), client, "s", SubnetUpdateOpts{AllocationPools: &pools}); err != nil {
		t.Fatalf("UpdateSubnet: %v", err)
	}
	name := "n"
	if err := UpdateSubnet(context.Background(), client, "s", SubnetUpdateOpts{Name: &name}); err != nil {
		t.Fatalf("UpdateSubnet: %v", err)
	}
	want := []string{
		`{"subnet":{"allocation_pools":[],"dns_nameservers":[]}}`,
		`{"subnet":{"allocation_pools":[{"end":"10.0.0.9","start":"10.0.0.2"}]}}`,
		`{"subnet":{"name":"n"}}`,
	}
	for i, w := range want {
		if bodies[i] != w {
			t.Errorf("body %d = %s, want %s", i, bodies[i], w)
		}
	}
}
