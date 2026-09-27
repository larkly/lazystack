package fippicker

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

var testTarget = network.FloatingIPTarget{PortID: "p1", IPAddress: "192.0.2.5", ExternalNetworkIDs: []string{"ext"}}

// fakeNeutron serves just enough of the Neutron API for allocateAndAssociate.
// It records every request as "METHOD /path".
type fakeNeutron struct {
	mu          sync.Mutex
	calls       []string
	associateOK bool
	releaseOK   bool
}

func (f *fakeNeutron) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.Method + " " + r.URL.Path {
	case "GET /networks":
		_, _ = w.Write([]byte(`{"networks":[{"id":"ext","name":"public","router:external":true}]}`))
	case "POST /floatingips":
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"floatingip":{"id":"new-fip","floating_ip_address":"203.0.113.10","floating_network_id":"ext"}}`))
	case "PUT /floatingips/new-fip":
		if !f.associateOK {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"floatingip":{"id":"new-fip","port_id":"p1"}}`))
	case "DELETE /floatingips/new-fip":
		if !f.releaseOK {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeNeutron) called(call string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c == call {
			return true
		}
	}
	return false
}

func TestAllocateReleasesFIPWhenAssociationFails(t *testing.T) {
	fake := &fakeNeutron{releaseOK: true}
	client, cleanup := testutil.FakeServiceClient(fake)
	defer cleanup()

	m := New(client, "server-1", "web")
	msg := m.allocateAndAssociate("ext", testTarget)()
	errMsg, ok := msg.(allocateErrMsg)
	if !ok {
		t.Fatalf("got %T, want allocateErrMsg", msg)
	}
	if !fake.called("PUT /floatingips/new-fip") || !fake.called("DELETE /floatingips/new-fip") {
		t.Fatalf("allocated FIP was not released; calls=%v", fake.calls)
	}
	if got := errMsg.err.Error(); !strings.Contains(got, "203.0.113.10") || !strings.Contains(got, "released") {
		t.Fatalf("error does not say the FIP was released: %v", got)
	}
}

func TestAllocateReportsLeftoverFIPWhenReleaseFails(t *testing.T) {
	fake := &fakeNeutron{}
	client, cleanup := testutil.FakeServiceClient(fake)
	defer cleanup()

	m := New(client, "server-1", "web")
	msg := m.allocateAndAssociate("ext", testTarget)()
	errMsg, ok := msg.(allocateErrMsg)
	if !ok {
		t.Fatalf("got %T, want allocateErrMsg", msg)
	}
	if !fake.called("PUT /floatingips/new-fip") || !fake.called("DELETE /floatingips/new-fip") {
		t.Fatalf("expected associate then release attempt; calls=%v", fake.calls)
	}
	if got := errMsg.err.Error(); !strings.Contains(got, "203.0.113.10") || !strings.Contains(got, "new-fip") {
		t.Fatalf("error does not identify the leftover FIP: %v", got)
	}
}

func TestAllocateAndAssociateSucceeds(t *testing.T) {
	fake := &fakeNeutron{associateOK: true}
	client, cleanup := testutil.FakeServiceClient(fake)
	defer cleanup()

	m := New(client, "server-1", "web")
	msg := m.allocateAndAssociate("ext", testTarget)()
	if done, ok := msg.(allocateDoneMsg); !ok || done.fipAddr != "203.0.113.10" {
		t.Fatalf("got %#v, want allocateDoneMsg", msg)
	}
	if fake.called("DELETE /floatingips/new-fip") {
		t.Fatal("FIP released after a successful association")
	}
}

func TestFetchOnlyOffersCurrentProjectFIPs(t *testing.T) {
	client, cleanup := testutil.FakeServiceClientWithFixture(`{"floatingips":[
		{"id":"mine","floating_ip_address":"2001:db8::1","tenant_id":"proj-a","project_id":"proj-a"},
		{"id":"other","floating_ip_address":"2001:db8::2","tenant_id":"proj-b","project_id":"proj-b"},
		{"id":"mine-bound","floating_ip_address":"2001:db8::3","tenant_id":"proj-a","project_id":"proj-a","port_id":"p9"},
		{"id":"mine-new-api","floating_ip_address":"2001:db8::4","project_id":"proj-a"},
		{"id":"other-new-api","floating_ip_address":"2001:db8::5","project_id":"proj-b"}
	]}`, "/floatingips")
	defer cleanup()

	m := New(client, "server-1", "web")
	m.projectID = "proj-a"
	msg := m.fetchUnassociatedFIPs()()
	loaded, ok := msg.(fipsLoadedMsg)
	if !ok {
		t.Fatalf("got %#v, want fipsLoadedMsg", msg)
	}
	var ids []string
	for _, f := range loaded.fips {
		ids = append(ids, f.ID)
	}
	if strings.Join(ids, ",") != "mine,mine-new-api" {
		t.Fatalf("offered FIPs = %v, want only unassociated FIPs of proj-a", ids)
	}
}

func TestFetchWithoutProjectIDOffersAllUnassociated(t *testing.T) {
	client, cleanup := testutil.FakeServiceClientWithFixture(`{"floatingips":[
		{"id":"a","floating_ip_address":"2001:db8::1","project_id":"proj-a"},
		{"id":"b","floating_ip_address":"2001:db8::2","project_id":"proj-b"}
	]}`, "/floatingips")
	defer cleanup()

	m := New(client, "server-1", "web")
	loaded, ok := m.fetchUnassociatedFIPs()().(fipsLoadedMsg)
	if !ok || len(loaded.fips) != 2 {
		t.Fatalf("got %#v, want both FIPs when the project is unknown", loaded)
	}
}

func TestProjectIDFromAuthResult(t *testing.T) {
	var r tokens.CreateResult
	r.Header = http.Header{"X-Subject-Token": {"tok"}}
	r.Body = map[string]interface{}{"token": map[string]interface{}{"project": map[string]interface{}{"id": "proj-a", "name": "a"}}}
	pc := &gophercloud.ProviderClient{}
	if err := pc.SetTokenAndAuthResult(r); err != nil {
		t.Fatal(err)
	}
	client := &gophercloud.ServiceClient{ProviderClient: pc}
	if got := New(client, "s", "web").projectID; got != "proj-a" {
		t.Fatalf("projectID = %q, want proj-a", got)
	}
	if got := New(nil, "s", "web").projectID; got != "" {
		t.Fatalf("projectID with nil client = %q, want empty", got)
	}
}

// A slow router lookup (admin credentials list every project's routers)
// must not use up the deadline of the external network listing, or
// "Allocate new" fails with a deadline error.
func TestSlowRouterLookupDoesNotStarveExternalNetworks(t *testing.T) {
	orig := shared.RequestTimeout
	shared.RequestTimeout = 200 * time.Millisecond
	t.Cleanup(func() { shared.RequestTimeout = orig })

	client, cleanup := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/ports":
			if r.URL.Query().Get("device_id") != "srv" {
				<-r.Context().Done() // router interfaces stall until the deadline
				return
			}
			_, _ = w.Write([]byte(`{"ports":[{"id":"p1","device_id":"srv","network_id":"net-a",
				"fixed_ips":[{"subnet_id":"sub-a","ip_address":"10.0.0.5"}]}]}`))
		case "/routers":
			_, _ = w.Write([]byte(`{"routers":[{"id":"r1","external_gateway_info":{"network_id":"ext-1"}}]}`))
		case "/networks":
			_, _ = w.Write([]byte(`{"networks":[{"id":"ext-1","name":"public","router:external":true}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer cleanup()

	msg := New(client, "srv", "web").resolveTargets(nil)()
	loaded, ok := msg.(targetsLoadedMsg)
	if !ok {
		t.Fatalf("got %#v, want targetsLoadedMsg", msg)
	}
	if loaded.extNetID != "ext-1" || len(loaded.targets) != 1 {
		t.Fatalf("extNetID=%q targets=%v", loaded.extNetID, loaded.targets)
	}
}
