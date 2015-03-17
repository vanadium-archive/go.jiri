package util

import (
	"runtime"
	"testing"
)

var empty = Platform{}

type parseTest struct {
	platform string
	expected Platform
}

// TestParsePlatform checks the program logic used for parsing
// platform strings.
func TestParsePlatform(t *testing.T) {
	tests := []parseTest{
		{"386", empty},
		{"386-linux-gnu-oops", empty},
		{"", Platform{runtime.GOARCH, "unknown", runtime.GOOS, "unknown"}},
		{"386-linux", Platform{"386", "", "linux", "unknown"}},
		{"386-linux-gnu", Platform{"386", "", "linux", "gnu"}},
		{"386v1.0-linux-gnu", Platform{"386", "v1.0", "linux", "gnu"}},
		{"armv1.0-linux-gnu", Platform{"arm", "v1.0", "linux", "gnu"}},
		{"amd64v1.0-linux-gnu", Platform{"amd64", "v1.0", "linux", "gnu"}},
		{"oops-linux", Platform{"unknown", "unknown", "linux", "unknown"}},
	}
	for _, test := range tests {
		got, err := ParsePlatform(test.platform)
		if test.expected == empty {
			if err == nil {
				t.Errorf("ParsePlatform(%v) didn't fail and it should", test.platform)
			}
			continue
		}
		if err != nil {
			t.Errorf("unexpected failure: %v", err)
			continue
		}
		if got != test.expected {
			t.Errorf("unexpected result: expected %+v, got %+v", test.expected, got)
		}
	}
}
