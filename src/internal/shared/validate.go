package shared

import (
	"net"
	"net/netip"
	"strings"
)

// Input validators shared by the forms, so bad addresses are caught before
// a request instead of surfacing as a raw API 400. All of them trim
// surrounding whitespace and accept IPv4 and IPv6 alike.

// parseAddr parses an IP address, rejecting zoned addresses (fe80::1%eth0),
// which OpenStack APIs do not accept.
func parseAddr(s string) (netip.Addr, bool) {
	a, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil || a.Zone() != "" {
		return netip.Addr{}, false
	}
	return a, true
}

// ValidIP reports whether s is a single IPv4 or IPv6 address.
func ValidIP(s string) bool {
	_, ok := parseAddr(s)
	return ok
}

// ValidCIDR reports whether s is an address/prefix-length pair such as
// 10.0.0.0/24 or 2001:db8::/64. Host bits may be set.
func ValidCIDR(s string) bool {
	_, err := netip.ParsePrefix(strings.TrimSpace(s))
	return err == nil
}

// IPInCIDR reports whether ip is a valid address inside cidr. An address of
// the other IP family is never inside.
func IPInCIDR(ip, cidr string) bool {
	a, ok := parseAddr(ip)
	if !ok {
		return false
	}
	p, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	return err == nil && p.Contains(a)
}

// ParseIPOrCIDR parses either a CIDR or a bare address; a bare address
// becomes a host prefix (/32 or /128).
func ParseIPOrCIDR(s string) (netip.Prefix, bool) {
	s = strings.TrimSpace(s)
	if p, err := netip.ParsePrefix(s); err == nil {
		return p, true
	}
	a, ok := parseAddr(s)
	if !ok {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(a, a.BitLen()), true
}

// ValidPort reports whether p is a TCP/UDP port number (1-65535).
func ValidPort(p int) bool {
	return p >= 1 && p <= 65535
}

// ValidMAC reports whether s is a 48-bit MAC address, such as
// fa:16:3e:00:00:01 or fa-16-3e-00-00-01.
func ValidMAC(s string) bool {
	hw, err := net.ParseMAC(strings.TrimSpace(s))
	return err == nil && len(hw) == 6
}
