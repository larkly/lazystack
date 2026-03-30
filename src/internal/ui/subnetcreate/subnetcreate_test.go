package subnetcreate

import (
	"testing"
)

func TestIPv6ModeValue(t *testing.T) {
	tests := []struct {
		idx  int
		want string
	}{
		{idx: 0, want: "slaac"},
		{idx: 1, want: "dhcpv6-stateful"},
		{idx: 2, want: "dhcpv6-stateless"},
		{idx: 3, want: ""},
		{idx: 99, want: ""},
	}

	for _, tc := range tests {
		if got := ipv6ModeValue(tc.idx); got != tc.want {
			t.Fatalf("ipv6ModeValue(%d)=%q want %q", tc.idx, got, tc.want)
		}
	}
}

