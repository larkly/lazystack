package portedit

import (
	"reflect"
	"strings"
	"testing"

	"github.com/larkly/lazystack/internal/network"
)

func TestParseAddressPairs(t *testing.T) {
	tests := []struct {
		raw     string
		want    []network.AddressPair
		wantErr string
	}{
		{raw: "", want: nil},
		{raw: "10.0.0.5", want: []network.AddressPair{{IPAddress: "10.0.0.5"}}},
		{raw: " 10.0.0.0/24 , fa:16:3e:00:00:01 ; 2001:db8::/64", want: []network.AddressPair{
			{IPAddress: "10.0.0.0/24", MACAddress: "fa:16:3e:00:00:01"},
			{IPAddress: "2001:db8::/64"},
		}},
		{raw: "10.0.0.300", wantErr: `"10.0.0.300" is not a valid IP or CIDR`},
		{raw: "2001:db8::/129", wantErr: "is not a valid IP or CIDR"},
		{raw: "10.0.0.5,fa:16:3e", wantErr: `"fa:16:3e" is not a valid MAC address`},
		{raw: "10.0.0.5; vip", wantErr: `address pair "vip"`},
	}
	for _, tc := range tests {
		got, err := parseAddressPairs(tc.raw)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("parseAddressPairs(%q) err = %v, want it to contain %q", tc.raw, err, tc.wantErr)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseAddressPairs(%q) = %v, %v; want %v", tc.raw, got, err, tc.want)
		}
	}
}

func TestSubmitRejectsInvalidAddressPair(t *testing.T) {
	var bodies []string
	m := loadedModel(t, &bodies)
	m.allowPairInput.SetValue("10.0.0.5,not-a-mac")
	m, cmd := m.submit()
	run(cmd)
	if len(bodies) != 0 || !strings.Contains(m.err, "is not a valid MAC address") {
		t.Fatalf("bodies=%q err=%q, want rejection before any request", bodies, m.err)
	}
}
