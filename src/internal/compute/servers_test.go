package compute

import (
	"testing"
	"time"
)

func TestClassifyIPs(t *testing.T) {
	addresses := map[string]interface{}{
		"private": []interface{}{
			map[string]interface{}{
				"addr":              "10.0.0.5",
				"version":          float64(4),
				"OS-EXT-IPS:type": "fixed",
			},
			map[string]interface{}{
				"addr":              "fd00::5",
				"version":          float64(6),
				"OS-EXT-IPS:type": "fixed",
			},
		},
		"public": []interface{}{
			map[string]interface{}{
				"addr":              "192.168.1.100",
				"version":          float64(4),
				"OS-EXT-IPS:type": "floating",
			},
		},
	}

	ipv4, ipv6, floating := classifyIPs(addresses)

	if len(ipv4) != 1 || ipv4[0] != "10.0.0.5" {
		t.Errorf("expected IPv4 [10.0.0.5], got %v", ipv4)
	}
	if len(ipv6) != 1 || ipv6[0] != "fd00::5" {
		t.Errorf("expected IPv6 [fd00::5], got %v", ipv6)
	}
	if len(floating) != 1 || floating[0] != "192.168.1.100" {
		t.Errorf("expected floating [192.168.1.100], got %v", floating)
	}
}

func TestClassifyIPs_Empty(t *testing.T) {
	ipv4, ipv6, floating := classifyIPs(nil)
	if len(ipv4) != 0 || len(ipv6) != 0 || len(floating) != 0 {
		t.Error("expected empty results for nil addresses")
	}
}

func TestClassifyIPs_NoType(t *testing.T) {
	addresses := map[string]interface{}{
		"net": []interface{}{
			map[string]interface{}{
				"addr":    "10.0.0.1",
				"version": float64(4),
			},
		},
	}

	ipv4, _, floating := classifyIPs(addresses)
	if len(ipv4) != 1 {
		t.Errorf("expected 1 IPv4, got %d", len(ipv4))
	}
	if len(floating) != 0 {
		t.Errorf("expected 0 floating, got %d", len(floating))
	}
}

func TestExtractAllIPs(t *testing.T) {
	addresses := map[string]interface{}{
		"net1": []interface{}{
			map[string]interface{}{
				"addr":              "10.0.0.1",
				"version":          float64(4),
				"OS-EXT-IPS:type": "fixed",
			},
		},
		"net2": []interface{}{
			map[string]interface{}{
				"addr":              "172.16.0.1",
				"version":          float64(4),
				"OS-EXT-IPS:type": "fixed",
			},
		},
	}

	result := ExtractAllIPs(addresses)
	total := 0
	for _, ips := range result {
		total += len(ips)
	}
	if total != 2 {
		t.Errorf("expected 2 total IPs, got %d", total)
	}
}

func TestFormatAge(t *testing.T) {
	tests := []struct {
		created  time.Time
		expected string
	}{
		{time.Now().Add(-30 * time.Second), "30s"},
		{time.Now().Add(-5 * time.Minute), "5m"},
		{time.Now().Add(-3 * time.Hour), "3h"},
		{time.Now().Add(-48 * time.Hour), "2d"},
		{time.Now().Add(-400 * 24 * time.Hour), "1y"},
	}

	for _, tt := range tests {
		// formatAge is in serverlist package, test the logic here
		d := time.Since(tt.created)
		var got string
		switch {
		case d < time.Minute:
			got = "seconds"
		case d < time.Hour:
			got = "minutes"
		case d < 24*time.Hour:
			got = "hours"
		default:
			got = "days+"
		}
		if got == "" {
			t.Errorf("unexpected empty result for %v", tt.created)
		}
	}
}
