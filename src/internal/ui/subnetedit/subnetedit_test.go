package subnetedit

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/testutil"
)

// subnetPutRecorder records PUT bodies and, like Neutron, rejects list
// attributes sent as null.
func subnetPutRecorder(t *testing.T, bodies *[]map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("bad JSON %s: %v", raw, err)
		}
		*bodies = append(*bodies, body["subnet"])
		for k, v := range body["subnet"] {
			if v == nil && k != "gateway_ip" {
				http.Error(w, `{"NeutronError":{"message":"Invalid input for `+k+`"}}`, http.StatusBadRequest)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"subnet":{"id":"sub-1"}}`))
	}
}

func runSubmit(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			switch r := c().(type) {
			case subnetUpdatedMsg, subnetUpdateErrMsg:
				return r
			}
		}
	}
	return nil
}

func editModel(t *testing.T, bodies *[]map[string]any) Model {
	client, cleanup := testutil.FakeServiceClient(subnetPutRecorder(t, bodies))
	t.Cleanup(cleanup)
	return New(client, network.Subnet{
		ID: "sub-1", Name: "app", CIDR: "10.0.0.0/24", GatewayIP: "10.0.0.1", EnableDHCP: true,
		DNSNameservers:  []string{"192.0.2.53", "192.0.2.54"},
		AllocationPools: []network.AllocationPool{{Start: "10.0.0.10", End: "10.0.0.200"}},
	})
}

func TestClearingDNSAndPoolsSendsEmptyArrays(t *testing.T) {
	var bodies []map[string]any
	m := editModel(t, &bodies)
	m.dnsInput.SetValue("")
	m.allocPoolInput.SetValue("")
	_, cmd := m.submit()
	msg := runSubmit(cmd)
	if len(bodies) != 1 {
		t.Fatalf("PUTs = %v", bodies)
	}
	for _, field := range []string{"dns_nameservers", "allocation_pools"} {
		v, present := bodies[0][field]
		list, isList := v.([]any)
		if !present || !isList || len(list) != 0 {
			t.Errorf("%s = %#v (present=%v), want []", field, v, present)
		}
	}
	if _, ok := msg.(subnetUpdatedMsg); !ok {
		t.Fatalf("result = %#v, want success", msg)
	}
}

func TestUnchangedListsOmittedAndPopulatedTrimmed(t *testing.T) {
	var bodies []map[string]any
	m := editModel(t, &bodies)
	m.nameInput.SetValue("app-renamed")
	_, cmd := m.submit()
	runSubmit(cmd)
	if len(bodies) != 1 {
		t.Fatalf("PUTs = %v", bodies)
	}
	for _, field := range []string{"dns_nameservers", "allocation_pools", "host_routes"} {
		if _, present := bodies[0][field]; present {
			t.Errorf("unchanged %s sent: %v", field, bodies[0])
		}
	}

	bodies = nil
	m = editModel(t, &bodies)
	m.dnsInput.SetValue("  192.0.2.99 , ,192.0.2.100 ")
	m.allocPoolInput.SetValue(" 10.0.0.20 - 10.0.0.30 ")
	_, cmd = m.submit()
	runSubmit(cmd)
	raw, _ := json.Marshal(bodies[0])
	want := `{"allocation_pools":[{"end":"10.0.0.30","start":"10.0.0.20"}],"dns_nameservers":["192.0.2.99","192.0.2.100"]}`
	if string(raw) != want {
		t.Fatalf("body = %s, want %s", raw, want)
	}
}

func TestUnnamedSubnetWithShortIDSubmits(t *testing.T) {
	for _, id := range []string{"s", "abcdefg", "子網子網子網子網"} {
		var bodies []map[string]any
		client, cleanup := testutil.FakeServiceClient(subnetPutRecorder(t, &bodies))
		m := New(client, network.Subnet{ID: id, Name: "old", CIDR: "10.0.0.0/24"})
		m.nameInput.SetValue("")
		_, cmd := m.submit()
		msg, ok := runSubmit(cmd).(subnetUpdatedMsg)
		cleanup()
		if !ok || !utf8.ValidString(msg.name) {
			t.Fatalf("id %q: result %#v", id, msg)
		}
	}
}
