package vault

import (
	"testing"
)

func TestFindResultFullPath(t *testing.T) {
	tests := []struct {
		name     string
		result   FindResult
		expected string
	}{
		{
			name:     "simple path and key",
			result:   FindResult{SecretPath: "secret/myapp", Key: "password"},
			expected: "secret/myapp/password",
		},
		{
			name:     "nested path",
			result:   FindResult{SecretPath: "secret/myapp/db", Key: "host"},
			expected: "secret/myapp/db/host",
		},
		{
			name:     "dotted key",
			result:   FindResult{SecretPath: "secret/config", Key: "db.password"},
			expected: "secret/config/db.password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.FullPath(); got != tt.expected {
				t.Errorf("FullPath() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestMatchKeys(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		pattern  string
		expected int
		keys     []string // expected matched keys
	}{
		{
			name:     "wildcard matches all",
			data:     map[string]any{"password": "s1", "host": "s2", "port": "s3"},
			pattern:  "*",
			expected: 3,
		},
		{
			name:     "prefix glob",
			data:     map[string]any{"password": "s1", "port": "s2", "host": "s3"},
			pattern:  "p*",
			expected: 2,
			keys:     []string{"password", "port"},
		},
		{
			name:     "question mark glob",
			data:     map[string]any{"a1": "v1", "a2": "v2", "ab": "v3", "abc": "v4"},
			pattern:  "a?",
			expected: 3,
			keys:     []string{"a1", "a2", "ab"},
		},
		{
			name:     "character class glob",
			data:     map[string]any{"a1": "v1", "b1": "v2", "c1": "v3", "d1": "v4"},
			pattern:  "[abc]1",
			expected: 3,
			keys:     []string{"a1", "b1", "c1"},
		},
		{
			name:     "no match",
			data:     map[string]any{"password": "s1", "host": "s2"},
			pattern:  "z*",
			expected: 0,
		},
		{
			name:     "exact match",
			data:     map[string]any{"password": "s1", "host": "s2"},
			pattern:  "password",
			expected: 1,
			keys:     []string{"password"},
		},
		{
			name:     "empty data",
			data:     map[string]any{},
			pattern:  "*",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var results []FindResult
			matchKeys("secret/test", tt.data, tt.pattern, &results)

			if len(results) != tt.expected {
				t.Errorf("matchKeys() returned %d results, want %d", len(results), tt.expected)
			}

			if tt.keys != nil {
				matched := make(map[string]bool)
				for _, r := range results {
					matched[r.Key] = true
				}
				for _, key := range tt.keys {
					if !matched[key] {
						t.Errorf("expected key %q in results, not found", key)
					}
				}
			}

			// Verify all results have the correct SecretPath
			for _, r := range results {
				if r.SecretPath != "secret/test" {
					t.Errorf("result SecretPath = %q, want %q", r.SecretPath, "secret/test")
				}
			}
		})
	}
}
