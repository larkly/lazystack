package network

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/larkly/lazystack/internal/testutil"
)

// A cloned rule restricted to an address group must keep that restriction;
// dropping it would open the rule to any source. Egress rules scoped to an
// address group are not the default egress rule and must be copied too.
func TestCloneSecurityGroupKeepsRemoteAddressGroup(t *testing.T) {
	var posted []map[string]any
	client, cleanup := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /security-groups/src":
			w.Write([]byte(`{"security_group":{"id":"src","security_group_rules":[
				{"direction":"egress","ethertype":"IPv4"},
				{"direction":"egress","ethertype":"IPv4","remote_address_group_id":"ag-out"},
				{"direction":"ingress","ethertype":"IPv4","protocol":"tcp","port_range_min":443,"port_range_max":443,"remote_address_group_id":"ag-in"}]}}`))
		case "POST /security-groups":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"security_group":{"id":"dst","name":"copy"}}`))
		case "POST /security-group-rules":
			var body map[string]map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			posted = append(posted, body["security_group_rule"])
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"security_group_rule":{"id":"created"}}`))
		case "GET /security-groups/dst":
			w.Write([]byte(`{"security_group":{"id":"dst","name":"copy"}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer cleanup()

	if _, err := CloneSecurityGroup(context.Background(), client, "src", "copy", ""); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted %d rules, want 2 (default egress skipped, address-group egress kept): %v", len(posted), posted)
	}
	if posted[0]["direction"] != "egress" || posted[0]["remote_address_group_id"] != "ag-out" {
		t.Errorf("egress rule = %v, want remote_address_group_id ag-out", posted[0])
	}
	if posted[1]["direction"] != "ingress" || posted[1]["remote_address_group_id"] != "ag-in" {
		t.Errorf("ingress rule = %v, want remote_address_group_id ag-in", posted[1])
	}
}

// A clone must keep null port/ICMP bounds unset and explicit 0 bounds as 0.
// The SDK types collapse both into 0 and omit 0 on create, which widened ICMP
// echo reply (type 0) to all ICMP and type 8 code 0 to type 8 any code.
func TestCloneSecurityGroupKeepsNullVsZeroBounds(t *testing.T) {
	var posted []map[string]any
	client, cleanup := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /security-groups/src":
			w.Write([]byte(`{"security_group":{"id":"src","security_group_rules":[
				{"direction":"ingress","ethertype":"IPv4","protocol":"icmp","port_range_min":0,"port_range_max":null},
				{"direction":"ingress","ethertype":"IPv4","protocol":"icmp","port_range_min":8,"port_range_max":0},
				{"direction":"ingress","ethertype":"IPv4","protocol":"icmp","port_range_min":null,"port_range_max":null},
				{"direction":"ingress","ethertype":"IPv6","protocol":"tcp","port_range_min":null,"port_range_max":null,"description":"any tcp"},
				{"direction":"ingress","ethertype":"IPv6","protocol":"tcp","port_range_min":0,"port_range_max":0}]}}`))
		case "POST /security-groups":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"security_group":{"id":"dst","name":"copy"}}`))
		case "POST /security-group-rules":
			var body map[string]map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			posted = append(posted, body["security_group_rule"])
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"security_group_rule":{"id":"created"}}`))
		case "GET /security-groups/dst":
			w.Write([]byte(`{"security_group":{"id":"dst","name":"copy"}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer cleanup()

	if _, err := CloneSecurityGroup(context.Background(), client, "src", "copy", ""); err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{
		{"security_group_id": "dst", "direction": "ingress", "ethertype": "IPv4", "protocol": "icmp", "port_range_min": float64(0)},
		{"security_group_id": "dst", "direction": "ingress", "ethertype": "IPv4", "protocol": "icmp", "port_range_min": float64(8), "port_range_max": float64(0)},
		{"security_group_id": "dst", "direction": "ingress", "ethertype": "IPv4", "protocol": "icmp"},
		{"security_group_id": "dst", "direction": "ingress", "ethertype": "IPv6", "protocol": "tcp", "description": "any tcp"},
		{"security_group_id": "dst", "direction": "ingress", "ethertype": "IPv6", "protocol": "tcp", "port_range_min": float64(0), "port_range_max": float64(0)},
	}
	if !reflect.DeepEqual(posted, want) {
		t.Errorf("posted rules:\n got %v\nwant %v", posted, want)
	}
}
