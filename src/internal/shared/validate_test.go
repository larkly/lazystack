package shared

import "testing"

func TestValidIP(t *testing.T) {
	cases := map[string]bool{
		"10.0.0.1":         true,
		"2001:db8::1":      true,
		" 10.0.0.1 ":       true,
		"::ffff:10.0.0.1":  true,
		"":                 false,
		"10.0.0.256":       false,
		"10.0.0.0/24":      false,
		"fe80::1%eth0":     false,
		"host.example.com": false,
		"2001:db8::1::2":   false,
	}
	for in, want := range cases {
		if got := ValidIP(in); got != want {
			t.Errorf("ValidIP(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestValidCIDR(t *testing.T) {
	cases := map[string]bool{
		"10.0.0.0/24":    true,
		"10.0.0.1/24":    true,
		"2001:db8::/64":  true,
		" 10.0.0.0/8 ":   true,
		"10.0.0.0":       false,
		"10.0.0.0/33":    false,
		"2001:db8::/129": false,
		"":               false,
		"garbage/24":     false,
	}
	for in, want := range cases {
		if got := ValidCIDR(in); got != want {
			t.Errorf("ValidCIDR(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestIPInCIDR(t *testing.T) {
	cases := []struct {
		ip, cidr string
		want     bool
	}{
		{"10.0.0.1", "10.0.0.0/24", true},
		{"10.0.1.1", "10.0.0.0/24", false},
		{"2001:db8::1", "2001:db8::/64", true},
		{"2001:db9::1", "2001:db8::/64", false},
		{"10.0.0.1", "2001:db8::/64", false},
		{"::ffff:10.0.0.1", "10.0.0.0/24", false},
		{"bad", "10.0.0.0/24", false},
		{"10.0.0.1", "bad", false},
		{" 10.0.0.1 ", " 10.0.0.0/24 ", true},
	}
	for _, c := range cases {
		if got := IPInCIDR(c.ip, c.cidr); got != c.want {
			t.Errorf("IPInCIDR(%q, %q) = %v, want %v", c.ip, c.cidr, got, c.want)
		}
	}
}

func TestParseIPOrCIDR(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"10.0.0.1", "10.0.0.1/32", true},
		{"10.0.0.0/24", "10.0.0.0/24", true},
		{"2001:db8::1", "2001:db8::1/128", true},
		{"2001:db8::/64", "2001:db8::/64", true},
		{"", "", false},
		{"10.0.0.0/40", "", false},
		{"fe80::1%eth0", "", false},
		{"nope", "", false},
	}
	for _, c := range cases {
		p, ok := ParseIPOrCIDR(c.in)
		if ok != c.ok || (ok && p.String() != c.want) {
			t.Errorf("ParseIPOrCIDR(%q) = %v, %v; want %s, %v", c.in, p, ok, c.want, c.ok)
		}
	}
}

func TestValidPort(t *testing.T) {
	cases := map[int]bool{0: false, 1: true, 80: true, 65535: true, 65536: false, -1: false}
	for in, want := range cases {
		if got := ValidPort(in); got != want {
			t.Errorf("ValidPort(%d) = %v, want %v", in, got, want)
		}
	}
}

func TestValidMAC(t *testing.T) {
	cases := map[string]bool{
		"fa:16:3e:00:00:01":       true,
		"FA:16:3E:00:00:01":       true,
		"fa-16-3e-00-00-01":       true,
		" fa:16:3e:00:00:01 ":     true,
		"fa:16:3e:00:00":          false,
		"fa:16:3e:00:00:01:02:03": false,
		"zz:16:3e:00:00:01":       false,
		"":                        false,
	}
	for in, want := range cases {
		if got := ValidMAC(in); got != want {
			t.Errorf("ValidMAC(%q) = %v, want %v", in, got, want)
		}
	}
}
