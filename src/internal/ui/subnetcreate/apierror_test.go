package subnetcreate

import (
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/larkly/lazystack/internal/testutil"
)

// A failed create shows the friendly message with Neutron's own reason,
// not gophercloud's raw text with the endpoint URL.
func TestCreateErrorIsSanitized(t *testing.T) {
	client, closeFn := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"NeutronError":{"type":"InvalidInput","message":"Invalid input for operation: overlapping CIDR."}}`))
	}))
	defer closeFn()

	m := New(client, "net", "net")
	m.cidrInput.SetValue("10.0.0.0/24")
	m, cmd := m.submit()
	if cmd == nil {
		t.Fatalf("submit rejected: %q", m.err)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("submit did not return a batch")
	}
	var result tea.Msg
	for _, c := range batch {
		if msg, ok := c().(subnetCreateErrMsg); ok {
			result = msg
		}
	}
	if result == nil {
		t.Fatal("no create error returned")
	}
	m, _ = m.Update(result)
	want := "Invalid request — check your input values and try again. (Invalid input for operation: overlapping CIDR.)"
	if m.err != want {
		t.Fatalf("err = %q, want %q", m.err, want)
	}
	if strings.Contains(m.err, "http") {
		t.Fatalf("err leaks the endpoint: %q", m.err)
	}
}
