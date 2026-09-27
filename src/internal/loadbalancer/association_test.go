package loadbalancer

import "testing"

func TestCompatiblePoolProtocol(t *testing.T) {
	for _, tc := range []struct {
		listener, pool string
		want           bool
	}{
		{"HTTP", "HTTP", true},
		{"HTTP", "PROXY", true},
		{"HTTP", "TCP", false},
		{"HTTPS", "TCP", true},
		{"TCP", "HTTP", true},
		{"TCP", "UDP", false},
		{"UDP", "UDP", true},
		{"UDP", "TCP", false},
		{"TERMINATED_HTTPS", "HTTP", true},
		{"UNKNOWN", "HTTP", false},
	} {
		if got := CompatiblePoolProtocol(tc.listener, tc.pool); got != tc.want {
			t.Errorf("CompatiblePoolProtocol(%s, %s) = %v, want %v", tc.listener, tc.pool, got, tc.want)
		}
	}
}
