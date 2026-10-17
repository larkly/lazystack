package compute

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/larkly/lazystack/internal/testutil"
)

// Nova 2.14 removed onSharedStorage from the evacuate schema and rejects any
// request that still carries it; older microversions require it.
func TestEvacuateOptsBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts evacuateOpts
		want map[string]any
	}{
		{"legacy", evacuateOpts{microversion: "2.13", host: "target", onSharedStorage: true},
			map[string]any{"host": "target", "onSharedStorage": true}},
		{"legacy no microversion", evacuateOpts{host: "target"},
			map[string]any{"host": "target", "onSharedStorage": false}},
		{"2.14", evacuateOpts{microversion: "2.14", host: "target", onSharedStorage: true},
			map[string]any{"host": "target"}},
		{"2.100 with admin pass", evacuateOpts{microversion: "2.100", host: "target", adminPass: "pw", onSharedStorage: true},
			map[string]any{"host": "target", "adminPass": "pw"}},
		{"2.100 scheduler picks host", evacuateOpts{microversion: "2.100"},
			map[string]any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := tc.opts.ToEvacuateMap()
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got["evacuate"], tc.want) {
				t.Errorf("body=%s, want evacuate=%v", raw, tc.want)
			}
		})
	}
}

func TestEvacuateServerOmitsSharedStorageFrom214(t *testing.T) {
	client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if _, ok := body["evacuate"]["onSharedStorage"]; ok {
			t.Errorf("onSharedStorage sent at 2.100: %v", body)
		}
		if body["evacuate"]["host"] != "target" {
			t.Errorf("host missing: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer close()
	client.Microversion = "2.100"
	if _, err := EvacuateServer(context.Background(), client, "vm", "target", true); err != nil {
		t.Fatal(err)
	}
}

func TestMicroversionAtLeast(t *testing.T) {
	for _, tc := range []struct {
		v    string
		want bool
	}{
		{"2.14", true}, {"2.100", true}, {"3.0", true},
		{"2.9", false}, {"2.13", false}, {"1.99", false},
		{"", false}, {"latest", false}, {"2.x", false},
	} {
		if got := microversionAtLeast(tc.v, 2, 14); got != tc.want {
			t.Errorf("microversionAtLeast(%q, 2, 14)=%v, want %v", tc.v, got, tc.want)
		}
	}
}
