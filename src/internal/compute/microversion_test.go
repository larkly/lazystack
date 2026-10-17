package compute

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/testutil"
)

func TestMicroversionAtLeast(t *testing.T) {
	for _, tc := range []struct {
		v, min string
		want   bool
	}{
		{"2.14", "2.14", true}, {"2.100", "2.14", true}, {"3.0", "2.14", true},
		{"2.9", "2.14", false}, {"2.13", "2.14", false}, {"1.99", "2.14", false},
		// Minors compare numerically, not as strings.
		{"2.10", "2.9", true}, {"2.9", "2.10", false}, {"2.89", "2.9", true},
		// Empty or malformed counts as the 2.1 base.
		{"", "2.14", false}, {"latest", "2.14", false}, {"2.x", "2.14", false},
		{"", "2.1", true}, {"latest", "2.1", true},
	} {
		if got := microversionAtLeast(tc.v, tc.min); got != tc.want {
			t.Errorf("microversionAtLeast(%q, %q)=%v, want %v", tc.v, tc.min, got, tc.want)
		}
	}
}

func TestRequireMicroversion(t *testing.T) {
	for _, tc := range []struct {
		connected, min string
		wantErr        string
	}{
		{"2.6", "2.6", ""},
		{"2.100", "2.6", ""},
		{"2.10", "2.9", ""},
		{"2.9", "2.10", "locked state requires Nova microversion 2.10; connected with 2.9"},
		{"2.5", "2.6", "locked state requires Nova microversion 2.6; connected with 2.5"},
		// No negotiated microversion means no header, i.e. Nova's 2.1 base.
		{"", "2.6", "locked state requires Nova microversion 2.6; connected with 2.1"},
	} {
		err := RequireMicroversion(&gophercloud.ServiceClient{Microversion: tc.connected}, tc.min, "locked state")
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("connected %q, min %q: unexpected error %v", tc.connected, tc.min, err)
		case tc.wantErr != "" && (err == nil || err.Error() != tc.wantErr):
			t.Errorf("connected %q, min %q: error %v, want %q", tc.connected, tc.min, err, tc.wantErr)
		}
	}
}

// Nova only serves POST /servers/{id}/remote-consoles from 2.6; below that
// the request fails with an opaque 404, so it must not be sent at all.
func TestGetRemoteConsoleRequiresMicroversion26(t *testing.T) {
	calls := 0
	client, closeFn := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.NotFound(w, r)
	}))
	defer closeFn()
	client.Microversion = "2.5"

	_, err := GetRemoteConsole(context.Background(), client, "vm")
	if err == nil || !strings.Contains(err.Error(), "remote console requires Nova microversion 2.6; connected with 2.5") {
		t.Fatalf("error = %v, want the microversion hint", err)
	}
	if calls != 0 {
		t.Errorf("%d request(s) sent below the microversion floor", calls)
	}
}

func TestGetRemoteConsoleAtMicroversion26(t *testing.T) {
	client, closeFn := testutil.FakeServiceClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/servers/vm/remote-consoles" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"remote_console":{"protocol":"vnc","type":"novnc","url":"https://console.example/vnc"}}`))
	}))
	defer closeFn()
	client.Microversion = "2.6"

	url, err := GetRemoteConsole(context.Background(), client, "vm")
	if err != nil || url != "https://console.example/vnc" {
		t.Fatalf("got (%q, %v), want the console URL", url, err)
	}
}
