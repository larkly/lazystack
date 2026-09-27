package subnetpicker

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/testutil"
)

// fakeNeutron serves the calls addInterfaceCmd makes when the router has no
// port on the subnet's network yet.
type fakeNeutron struct {
	mu           sync.Mutex
	createStatus int
	addStatus    int
	deleteStatus int
	deletes      []string
}

func (n *fakeNeutron) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n.mu.Lock()
	defer n.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/ports":
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ports":[]}`)
	case r.Method == http.MethodPost && r.URL.Path == "/ports":
		if n.createStatus != 0 {
			http.Error(w, "create refused", n.createStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"port":{"id":"port-orphan-7","network_id":"net-1","fixed_ips":[{"subnet_id":"sub-1","ip_address":"10.0.0.5"}]}}`)
	case r.Method == http.MethodPut && r.URL.Path == "/routers/router-1/add_router_interface":
		if n.addStatus != 0 {
			http.Error(w, "interface refused", n.addStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"router-1","port_id":"port-orphan-7","subnet_id":"sub-1"}`)
	case r.Method == http.MethodDelete && r.URL.Path == "/ports/port-orphan-7":
		n.deletes = append(n.deletes, "port-orphan-7")
		if n.deleteStatus != 0 {
			http.Error(w, "delete refused", n.deleteStatus)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}
}

func runAddInterface(t *testing.T, n *fakeNeutron) interfaceAddErrMsg {
	t.Helper()
	client, cleanup := testutil.FakeServiceClient(n)
	t.Cleanup(cleanup)
	m := New(client, "router-1", "edge")
	sub := network.Subnet{ID: "sub-1", NetworkID: "net-1", CIDR: "10.0.0.0/24"}
	msg := m.addInterfaceCmd(client, "router-1", "edge", sub, "10.0.0.5")()
	errMsg, ok := msg.(interfaceAddErrMsg)
	if !ok {
		t.Fatalf("result = %#v, want interfaceAddErrMsg", msg)
	}
	return errMsg
}

// #314: interface failure plus failed port cleanup keeps the primary error
// and names the orphaned port.
func TestAddInterfaceReportsOrphanPortWhenCleanupFails(t *testing.T) {
	n := &fakeNeutron{addStatus: http.StatusConflict, deleteStatus: http.StatusInternalServerError}
	text := runAddInterface(t, n).err.Error()
	if !strings.Contains(text, "adding port interface to router") {
		t.Fatalf("error %q lost the primary interface failure", text)
	}
	if !strings.Contains(text, "port-orphan-7") || !strings.Contains(text, "cleanup") {
		t.Fatalf("error %q does not name the orphaned port and cleanup failure", text)
	}
}

// #314: a successful cleanup leaves the primary error unchanged.
func TestAddInterfaceKeepsPrimaryErrorWhenCleanupSucceeds(t *testing.T) {
	n := &fakeNeutron{addStatus: http.StatusConflict}
	text := runAddInterface(t, n).err.Error()
	if strings.Contains(text, "cleanup") || !strings.Contains(text, "adding port interface to router") {
		t.Fatalf("error = %q, want the unchanged interface failure", text)
	}
	if len(n.deletes) != 1 {
		t.Fatalf("deletes = %v, want one cleanup", n.deletes)
	}
}

// #314: no cleanup runs when the port was never created.
func TestAddInterfaceNoCleanupWhenPortCreateFails(t *testing.T) {
	n := &fakeNeutron{createStatus: http.StatusConflict}
	runAddInterface(t, n)
	if len(n.deletes) != 0 {
		t.Fatalf("deletes = %v, want none", n.deletes)
	}
}
