package gotoolchain

import (
	"testing"

	"github.com/Masterminds/semver/v3"
)

func TestNormalizeGoVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Standard versions
		{"1.23", "1.23.0"},
		{"1.23.1", "1.23.1"},
		{"1.22.0", "1.22.0"},

		// With go prefix
		{"go1.23", "1.23.0"},
		{"go1.23.1", "1.23.1"},
		{"go1.22.0", "1.22.0"},

		// Prerelease versions
		{"1.23rc1", "1.23.0-rc.1"},
		{"1.23beta2", "1.23.0-beta.2"},
		{"1.23alpha1", "1.23.0-alpha.1"},

		// Prerelease with go prefix
		{"go1.23rc1", "1.23.0-rc.1"},
		{"go1.23beta2", "1.23.0-beta.2"},
		{"go1.23alpha1", "1.23.0-alpha.1"},

		// Edge cases - non-matching patterns fall through
		{"invalid", "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeGoVersion(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeGoVersion(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseGoVersionFunc(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{"1.23", "1.23.0", false},
		{"1.23.1", "1.23.1", false},
		{"go1.23", "1.23.0", false},
		{"go1.23.1", "1.23.1", false},
		{"1.23rc1", "1.23.0-rc.1", false},
		{"go1.23rc1", "1.23.0-rc.1", false},
		{"1.23beta2", "1.23.0-beta.2", false},
		{"go1.23beta2", "1.23.0-beta.2", false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, err := ParseGoVersion(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseGoVersion(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseGoVersion(%q) error = %v", tt.input, err)
				return
			}
			if v.String() != tt.expected {
				t.Errorf("ParseGoVersion(%q) = %s, want %s", tt.input, v.String(), tt.expected)
			}
		})
	}
}

func TestFormatGoDirective(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Stable versions
		{"1.23.0", "1.23"},
		{"1.23.1", "1.23.1"},
		{"1.22.0", "1.22"},
		// Prerelease versions: major.minor + prerelease (no patch)
		{"1.23.0-rc.1", "1.23rc1"},
		{"1.23.0-beta.2", "1.23beta2"},
		{"1.24.0-alpha.1", "1.24alpha1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, err := semver.NewVersion(tt.input)
			if err != nil {
				t.Fatalf("failed to parse version %q: %v", tt.input, err)
			}
			result := FormatGoDirective(v)
			if result != tt.expected {
				t.Errorf("FormatGoDirective(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatToolchainDirective(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Stable versions
		{"1.23.0", "go1.23.0"},
		{"1.23.1", "go1.23.1"},
		{"1.22.0", "go1.22.0"},
		// Prerelease versions: go + major.minor + prerelease (no patch)
		{"1.23.0-rc.1", "go1.23rc1"},
		{"1.23.0-beta.2", "go1.23beta2"},
		{"1.24.0-alpha.1", "go1.24alpha1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, err := semver.NewVersion(tt.input)
			if err != nil {
				t.Fatalf("failed to parse version %q: %v", tt.input, err)
			}
			result := FormatToolchainDirective(v)
			if result != tt.expected {
				t.Errorf("FormatToolchainDirective(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that parsing and formatting produces consistent results
	testCases := []struct {
		goInput           string
		expectedToolchain string
		expectedDirective string
	}{
		// Stable versions
		{"go1.23.0", "go1.23.0", "1.23"},
		{"go1.23.1", "go1.23.1", "1.23.1"},
		// Prerelease: input go1.23rc1 -> parsed 1.23.0-rc.1 -> formatted go1.23rc1
		{"go1.23rc1", "go1.23rc1", "1.23rc1"},
		{"go1.23beta2", "go1.23beta2", "1.23beta2"},
	}

	for _, tc := range testCases {
		t.Run(tc.goInput, func(t *testing.T) {
			v, err := ParseGoVersion(tc.goInput)
			if err != nil {
				t.Fatalf("ParseGoVersion(%q) error = %v", tc.goInput, err)
			}

			toolchainResult := FormatToolchainDirective(v)
			if toolchainResult != tc.expectedToolchain {
				t.Errorf("FormatToolchainDirective after parse = %q, want %q", toolchainResult, tc.expectedToolchain)
			}

			directiveResult := FormatGoDirective(v)
			if directiveResult != tc.expectedDirective {
				t.Errorf("FormatGoDirective after parse = %q, want %q", directiveResult, tc.expectedDirective)
			}
		})
	}
}
