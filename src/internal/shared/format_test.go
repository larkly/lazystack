package shared

import (
	"testing"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{"zero", 0, "-"},
		{"bytes", 500, "500 B"},
		{"one_kb", 1024, "1 KB"},
		{"kb_fractional", 1536, "2 KB"},
		{"one_mb", 1048576, "1.0 MB"},
		{"mb_fractional", 1572864, "1.5 MB"},
		{"one_gb", 1073741824, "1.0 GB"},
		{"gb_fractional", 1610612736, "1.5 GB"},
		{"large_gb", 10737418240, "10.0 GB"},
		{"just_below_kb", 1023, "1023 B"},
		{"just_below_mb", 1048575, "1024 KB"},
		{"just_below_gb", 1073741823, "1024.0 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSize(tt.bytes)
			if got != tt.expected {
				t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.expected)
			}
		})
	}
}
