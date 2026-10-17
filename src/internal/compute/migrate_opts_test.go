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

// From 2.25 Nova accepts block_migration "auto" and drops disk_over_commit;
// a hard-coded false rejects instances on local storage. Below 2.25 both
// booleans are required by the schema.
func TestLiveMigrateServerBody(t *testing.T) {
	for _, tc := range []struct {
		microversion string
		want         map[string]any
	}{
		{"", map[string]any{"host": "target", "block_migration": false, "disk_over_commit": false}},
		{"2.24", map[string]any{"host": "target", "block_migration": false, "disk_over_commit": false}},
		{"2.25", map[string]any{"host": "target", "block_migration": "auto"}},
		{"2.100", map[string]any{"host": "target", "block_migration": "auto"}},
	} {
		t.Run("v"+tc.microversion, func(t *testing.T) {
			client, close := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/servers/vm/action" {
					t.Errorf("request=%s %s", r.Method, r.URL.Path)
				}
				var body map[string]map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if !reflect.DeepEqual(body["os-migrateLive"], tc.want) {
					t.Errorf("body=%v, want os-migrateLive=%v", body, tc.want)
				}
				w.WriteHeader(http.StatusAccepted)
			}))
			defer close()
			client.Microversion = tc.microversion
			if err := LiveMigrateServer(context.Background(), client, "vm", "target"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLiveMigrateOptsEmptyHostIsNull(t *testing.T) {
	m, err := liveMigrateOpts{microversion: "2.100"}.ToLiveMigrateMap()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(m)
	if string(raw) != `{"os-migrateLive":{"block_migration":"auto","host":null}}` {
		t.Errorf("body=%s, want host null so the scheduler chooses", raw)
	}
}
