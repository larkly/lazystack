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
	client := fakeNeutronClient(bodyRecorder("port", &bodies))
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
