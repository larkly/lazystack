package selfupdate

import (
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
		want  []int
	}{
		{"v1.2.3", []int{1, 2, 3}},
		{"0.1.0", []int{0, 1, 0}},
		{"v0.0.0", []int{0, 0, 0}},
		{"v0.3.0-7-g09160b8", []int{0, 3, 0}},
		{"v1.2.3-rc1", []int{1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseVersion(tt.input)
			if got == nil {
				t.Fatalf("parseVersion(%q) = nil, want %v", tt.input, tt.want)
			}
			for i, v := range tt.want {
				if got[i] != v {
					t.Errorf("parseVersion(%q)[%d] = %d, want %d", tt.input, i, got[i], v)
				}
			}
		})
	}
}

func TestParseVersion_Invalid(t *testing.T) {
	for _, input := range []string{"dev", "", "v1.2", "v1.2.3.4", "v1.x.3", "abc"} {
		t.Run(input, func(t *testing.T) {
			if parseVersion(input) != nil {
				t.Errorf("parseVersion(%q) should be nil", input)
			}
		})
	}
}

func TestJsonString(t *testing.T) {
	json := `{"tag_name": "v0.1.1", "name": "Release v0.1.1"}`
	tests := []struct {
		key, want string
	}{
		{"tag_name", "v0.1.1"},
		{"name", "Release v0.1.1"},
		{"missing", ""},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := jsonString(json, tt.key)
			if got != tt.want {
				t.Errorf("jsonString(json, %q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestJsonArray(t *testing.T) {
	json := `{"assets": [{"name": "bin-linux"}, {"name": "bin-darwin"}]}`
	items := jsonArray(json, "assets")
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if got := jsonString(items[0], "name"); got != "bin-linux" {
		t.Errorf("first asset name = %q, want %q", got, "bin-linux")
	}
	if got := jsonString(items[1], "name"); got != "bin-darwin" {
		t.Errorf("second asset name = %q, want %q", got, "bin-darwin")
	}
}

func TestJsonArray_Missing(t *testing.T) {
	items := jsonArray(`{"other": []}`, "assets")
	if items != nil {
		t.Errorf("expected nil for missing key, got %v", items)
	}
}

func TestCheckLatest_DevBuild(t *testing.T) {
	_, _, _, err := CheckLatest("dev")
	if err == nil {
		t.Error("expected error for dev build")
	}
}
