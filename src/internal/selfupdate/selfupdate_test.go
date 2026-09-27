package selfupdate

import (
	"context"
	"strings"
	"testing"
)

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"v0.1.1", "v0.0.1", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.2.0", "v0.1.9", true},
		{"v0.1.1", "v0.1.1", false},
		{"v0.1.0", "v0.1.1", false},
		{"v0.0.1", "v0.1.0", false},
		{"v1.0.0", "v1.0.0", false},
		{"v2.0.0", "v1.99.99", true},
		{"v0.3.1", "v0.3.0-7-g09160b8", true},
		// A git-describe build is the tag it was built from, not a
		// pre-release of it.
		{"v0.3.0", "v0.3.0-7-g09160b8", false},
		{"v0.3.0", "v0.3.0-7-g09160b8-dirty", false},
		{"v0.3.0", "v0.3.0-dirty", false},
		// Pre-releases (semver precedence).
		{"v0.13.0", "v0.13.0-rc1", true},
		{"v0.13.0-rc1", "v0.13.0", false},
		{"v0.13.0-rc1", "v0.12.0", true},
		{"v0.12.0", "v0.13.0-rc1", false},
		{"v0.13.0-rc2", "v0.13.0-rc1", true},
		{"v0.13.0-rc1", "v0.13.0-rc1", false},
		{"v0.13.0-rc.10", "v0.13.0-rc.2", true}, // numeric identifiers compare numerically
		{"v0.13.0-rc.2", "v0.13.0-rc.10", false},
		{"v0.13.0-beta", "v0.13.0-alpha", true},
		{"v0.13.0-alpha", "v0.13.0-1", true}, // alphanumeric > numeric
		{"v0.13.0-1", "v0.13.0-alpha", false},
		{"v0.13.0-alpha.1", "v0.13.0-alpha", true}, // longer list wins on a tie
		{"v0.13.0-alpha", "v0.13.0-alpha.1", false},
		{"v0.13.0", "v0.13.0-rc1-3-gabc1234", true},
		{"v0.13.0-rc2", "v0.13.0-rc1-3-gabc1234", true},
		{"v1.0.0+build.5", "v1.0.0", false}, // build metadata is ignored
		{"v1.0.0-rc.99999999999999999999", "v1.0.0-rc.9", true},
	}
	for _, tt := range tests {
		t.Run(tt.latest+"_vs_"+tt.current, func(t *testing.T) {
			got := isNewer(tt.latest, tt.current)
			if got != tt.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestIsNewer_Invalid(t *testing.T) {
	tests := []struct {
		latest, current string
	}{
		{"dev", "v0.1.0"},
		{"v0.1.0", "dev"},
		{"", "v0.1.0"},
		{"v0.1", "v0.1.0"},
		{"abc", "def"},
	}
	for _, tt := range tests {
		t.Run(tt.latest+"_vs_"+tt.current, func(t *testing.T) {
			if isNewer(tt.latest, tt.current) {
				t.Errorf("isNewer(%q, %q) should be false for invalid versions", tt.latest, tt.current)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		core  [3]int
		pre   string
	}{
		{"v1.2.3", [3]int{1, 2, 3}, ""},
		{"0.1.0", [3]int{0, 1, 0}, ""},
		{"v0.0.0", [3]int{0, 0, 0}, ""},
		{"v0.3.0-7-g09160b8", [3]int{0, 3, 0}, ""},
		{"v0.3.0-7-g09160b8-dirty", [3]int{0, 3, 0}, ""},
		{"v1.2.3-rc1", [3]int{1, 2, 3}, "rc1"},
		{"v1.2.3-rc.1", [3]int{1, 2, 3}, "rc.1"},
		{"v1.2.3-rc-1", [3]int{1, 2, 3}, "rc-1"},
		{"v1.2.3-rc1-4-gdeadbeef", [3]int{1, 2, 3}, "rc1"},
		{"v1.2.3+meta", [3]int{1, 2, 3}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseVersion(tt.input)
			if got == nil {
				t.Fatalf("parseVersion(%q) = nil, want %v-%s", tt.input, tt.core, tt.pre)
			}
			if got.core != tt.core || strings.Join(got.pre, ".") != tt.pre {
				t.Errorf("parseVersion(%q) = %v %q, want %v %q", tt.input, got.core, got.pre, tt.core, tt.pre)
			}
		})
	}
}

func TestParseVersion_Invalid(t *testing.T) {
	for _, input := range []string{"dev", "", "v1.2", "v1.2.3.4", "v1.x.3", "abc", "v+1.2.3", "v1.2.3-", "v1.2.3-rc..1", "v1.2.3-rc_1", "v1.2.3-rc\n1", "09160b8-dirty"} {
		t.Run(input, func(t *testing.T) {
			if parseVersion(input) != nil {
				t.Errorf("parseVersion(%q) should be nil", input)
			}
		})
	}
}

func TestCheckLatest_DevBuild(t *testing.T) {
	_, _, _, err := CheckLatest(context.Background(), "dev")
	if err == nil {
		t.Error("expected error for dev build")
	}
}
