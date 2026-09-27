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
	client := fakeNeutronClient(fipUpdateRecorder(t, &bodies))
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
	client := fakeNeutronClient(fipUpdateRecorder(t, &bodies))
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
