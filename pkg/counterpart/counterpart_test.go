package counterpart

import (
	"testing"
)

func TestDeriveFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "secrets suffix",
			input:    "/path/to/app-secrets.yaml",
			expected: "/path/to/app.yaml",
		},
		{
			name:     "enc suffix strips to first dot",
			input:    "/path/to/values.enc.yaml",
			expected: "/path/to/values.yaml",
		},
		{
			name:     "no special suffix uses first dot",
			input:    "/path/to/config.yaml",
			expected: "/path/to/config.yaml",
		},
		{
			name:     "always outputs .yaml extension",
			input:    "/path/to/app-secrets.yml",
			expected: "/path/to/app.yaml",
		},
		{
			name:     "secrets suffix takes precedence",
			input:    "/path/to/my.app-secrets.yaml",
			expected: "/path/to/my.app.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeriveFilename(tt.input)
			if result != tt.expected {
				t.Errorf("DeriveFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCleanFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple filename",
			input:    "/path/to/app-secrets.yaml",
			expected: "app",
		},
		{
			name:     "no path",
			input:    "config-secrets.yaml",
			expected: "config",
		},
		{
			name:     "enc suffix",
			input:    "values.enc.yaml",
			expected: "values",
		},
		{
			name:     "no special suffix",
			input:    "myfile.yaml",
			expected: "myfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanFilename(tt.input)
			if result != tt.expected {
				t.Errorf("CleanFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatRef(t *testing.T) {
	tests := []struct {
		name      string
		vaultPath string
		key       string
		expected  string
	}{
		{
			name:      "simple key",
			vaultPath: "secret/myapp",
			key:       "password",
			expected:  "ref+vault://secret/myapp/password#value",
		},
		{
			name:      "nested key",
			vaultPath: "secret/myapp",
			key:       "database.password",
			expected:  "ref+vault://secret/myapp/database.password#value",
		},
		{
			name:      "deep path",
			vaultPath: "secret/prod/myapp/config",
			key:       "api.key",
			expected:  "ref+vault://secret/prod/myapp/config/api.key#value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatRef(tt.vaultPath, tt.key)
			if result != tt.expected {
				t.Errorf("FormatRef(%q, %q) = %q, want %q", tt.vaultPath, tt.key, result, tt.expected)
			}
		})
	}
}
