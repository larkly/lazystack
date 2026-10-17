package network

import (
	"context"
	"encoding/json"
	"github.com/larkly/lazystack/internal/testutil"
	"net/http"
	"reflect"
	"testing"
)

func TestCreateSubnetOptionalFieldsHTTP(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/subnets" {
					t.Errorf("request %s %s", r.Method, r.URL.Path)
				}
				var body map[string]map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				want := map[string]any{"network_id": "net", "name": "v6", "ip_version": float64(6), "gateway_ip": "2001:db8::1", "enable_dhcp": false, "subnetpool_id": "pool", "prefixlen": float64(64), "ipv6_address_mode": "slaac", "ipv6_ra_mode": "slaac"}
				if !reflect.DeepEqual(body["subnet"], want) {
					t.Errorf("body=%v want=%v", body, want)
				}
				w.Header().Set("Content-Type", "application/json")
				if fail {
					w.WriteHeader(400)
					return
				}
				w.WriteHeader(201)
				w.Write([]byte(`{"subnet":{"id":"sub","name":"v6","network_id":"net","cidr":"2001:db8::/64","gateway_ip":"2001:db8::1","ip_version":6,"enable_dhcp":false,"allocation_pools":[{"start":"2001:db8::2","end":"2001:db8::ff"}]}}`))
			}))
			defer close()
			got, err := CreateSubnet(context.Background(), client, SubnetCreateOpts{NetworkID: "net", Name: "v6", IPVersion: 6, GatewayIP: ptr("2001:db8::1"), SubnetPoolID: "pool", PrefixLen: 64, IPv6AddressMode: "slaac", IPv6RAMode: "slaac"})
			if fail {
				if err == nil || got != nil {
					t.Fatalf("got=%v err=%v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := &Subnet{ID: "sub", Name: "v6", NetworkID: "net", CIDR: "2001:db8::/64", GatewayIP: "2001:db8::1", IPVersion: 6, AllocationPools: []AllocationPool{{Start: "2001:db8::2", End: "2001:db8::ff"}}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got=%+v want=%+v", got, want)
			}
		})
	}
}

func ptr(s string) *string { return &s }

// A nil gateway omits gateway_ip so Neutron assigns the first address; a
// pointer to "" must be sent as null, which creates a subnet without one.
func TestCreateSubnetGatewayIP(t *testing.T) {
	for _, tc := range []struct {
		name    string
		gateway *string
		want    string // raw JSON of gateway_ip; "" means absent
	}{
		{"auto", nil, ""},
		{"none", ptr(""), "null"},
		{"explicit", ptr("10.0.0.254"), `"10.0.0.254"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, closeFn := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Subnet map[string]json.RawMessage `json:"subnet"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				got, present := body.Subnet["gateway_ip"]
				switch {
				case tc.want == "" && present:
					t.Errorf("gateway_ip = %s, want it omitted", got)
				case tc.want != "" && string(got) != tc.want:
					t.Errorf("gateway_ip = %q (present=%v), want %s", got, present, tc.want)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(201)
				w.Write([]byte(`{"subnet":{"id":"sub","network_id":"net","cidr":"10.0.0.0/24","ip_version":4}}`))
			}))
			defer closeFn()
			if _, err := CreateSubnet(context.Background(), client, SubnetCreateOpts{NetworkID: "net", CIDR: "10.0.0.0/24", IPVersion: 4, GatewayIP: tc.gateway}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
