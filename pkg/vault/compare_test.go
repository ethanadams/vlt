package vault

import (
	"testing"
)

func TestParseVersionedPath(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedPath string
		expectedSpec VersionSpec
	}{
		{
			name:         "no version",
			input:        "secret/myapp/config",
			expectedPath: "secret/myapp/config",
			expectedSpec: VersionSpec{},
		},
		{
			name:         "specific version",
			input:        "secret/myapp/config@3",
			expectedPath: "secret/myapp/config",
			expectedSpec: VersionSpec{Version: 3},
		},
		{
			name:         "prev alias",
			input:        "secret/myapp/config@prev",
			expectedPath: "secret/myapp/config",
			expectedSpec: VersionSpec{IsPrev: true},
		},
		{
			name:         "previous alias",
			input:        "secret/myapp/config@previous",
			expectedPath: "secret/myapp/config",
			expectedSpec: VersionSpec{IsPrev: true},
		},
		{
			name:         "changes ago",
			input:        "secret/myapp@-3",
			expectedPath: "secret/myapp",
			expectedSpec: VersionSpec{IsChangesAgo: true, ChangesAgo: 3},
		},
		{
			name:         "version 1",
			input:        "secret/app@1",
			expectedPath: "secret/app",
			expectedSpec: VersionSpec{Version: 1},
		},
		{
			name:         "invalid version ignored",
			input:        "secret/app@invalid",
			expectedPath: "secret/app@invalid",
			expectedSpec: VersionSpec{},
		},
		{
			name:         "version 0 ignored",
			input:        "secret/app@0",
			expectedPath: "secret/app@0",
			expectedSpec: VersionSpec{},
		},
		{
			name:         "path with @ in name",
			input:        "secret/email@domain.com",
			expectedPath: "secret/email@domain.com",
			expectedSpec: VersionSpec{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, spec := ParseVersionedPath(tt.input)
			if path != tt.expectedPath {
				t.Errorf("path = %q, want %q", path, tt.expectedPath)
			}
			if spec != tt.expectedSpec {
				t.Errorf("spec = %+v, want %+v", spec, tt.expectedSpec)
			}
		})
	}
}

func TestVersionSpecHasVersion(t *testing.T) {
	tests := []struct {
		name     string
		spec     VersionSpec
		expected bool
	}{
		{"empty", VersionSpec{}, false},
		{"version set", VersionSpec{Version: 3}, true},
		{"prev set", VersionSpec{IsPrev: true}, true},
		{"changes ago set", VersionSpec{IsChangesAgo: true, ChangesAgo: 2}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.HasVersion(); got != tt.expected {
				t.Errorf("HasVersion() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCompareSecrets(t *testing.T) {
	tests := []struct {
		name      string
		secrets1  map[string]any
		secrets2  map[string]any
		wantOnly1 int
		wantOnly2 int
		wantDiff  int
		wantSame  int
	}{
		{
			name:      "identical",
			secrets1:  map[string]any{"a": "1", "b": "2"},
			secrets2:  map[string]any{"a": "1", "b": "2"},
			wantOnly1: 0,
			wantOnly2: 0,
			wantDiff:  0,
			wantSame:  2,
		},
		{
			name:      "only in first",
			secrets1:  map[string]any{"a": "1", "b": "2"},
			secrets2:  map[string]any{"a": "1"},
			wantOnly1: 1,
			wantOnly2: 0,
			wantDiff:  0,
			wantSame:  1,
		},
		{
			name:      "only in second",
			secrets1:  map[string]any{"a": "1"},
			secrets2:  map[string]any{"a": "1", "b": "2"},
			wantOnly1: 0,
			wantOnly2: 1,
			wantDiff:  0,
			wantSame:  1,
		},
		{
			name:      "different values",
			secrets1:  map[string]any{"a": "old"},
			secrets2:  map[string]any{"a": "new"},
			wantOnly1: 0,
			wantOnly2: 0,
			wantDiff:  1,
			wantSame:  0,
		},
		{
			name:      "empty maps",
			secrets1:  map[string]any{},
			secrets2:  map[string]any{},
			wantOnly1: 0,
			wantOnly2: 0,
			wantDiff:  0,
			wantSame:  0,
		},
		{
			name:      "complex scenario",
			secrets1:  map[string]any{"a": "1", "b": "old", "c": "3"},
			secrets2:  map[string]any{"a": "1", "b": "new", "d": "4"},
			wantOnly1: 1, // c
			wantOnly2: 1, // d
			wantDiff:  1, // b
			wantSame:  1, // a
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareSecrets(tt.secrets1, tt.secrets2)

			if got := len(result.OnlyInFirst); got != tt.wantOnly1 {
				t.Errorf("OnlyInFirst count = %d, want %d", got, tt.wantOnly1)
			}
			if got := len(result.OnlyInSecond); got != tt.wantOnly2 {
				t.Errorf("OnlyInSecond count = %d, want %d", got, tt.wantOnly2)
			}
			if got := len(result.Changed); got != tt.wantDiff {
				t.Errorf("Changed count = %d, want %d", got, tt.wantDiff)
			}
			if got := result.Unchanged; got != tt.wantSame {
				t.Errorf("Unchanged count = %d, want %d", got, tt.wantSame)
			}
		})
	}
}

func TestHashValue(t *testing.T) {
	// Same value should produce same hash
	h1 := hashValue("test-value")
	h2 := hashValue("test-value")
	if h1 != h2 {
		t.Error("same value produced different hashes")
	}

	// Different values should produce different hashes
	h3 := hashValue("different-value")
	if h1 == h3 {
		t.Error("different values produced same hash")
	}
}
