package vault

import (
	"testing"
)

func TestRestoreResultHasChanges(t *testing.T) {
	tests := []struct {
		name     string
		result   RestoreResult
		expected bool
	}{
		{
			name:     "no changes",
			result:   RestoreResult{},
			expected: false,
		},
		{
			name:     "with added",
			result:   RestoreResult{Added: []string{"a"}},
			expected: true,
		},
		{
			name:     "with updated",
			result:   RestoreResult{Updated: []string{"a"}},
			expected: true,
		},
		{
			name:     "with deleted",
			result:   RestoreResult{Deleted: []string{"a"}},
			expected: true,
		},
		{
			name:     "unchanged only",
			result:   RestoreResult{Unchanged: []string{"a", "b"}},
			expected: false,
		},
		{
			name:     "skipped only",
			result:   RestoreResult{Skipped: []string{"a"}},
			expected: false,
		},
		{
			name: "mixed",
			result: RestoreResult{
				Added:     []string{"a"},
				Unchanged: []string{"b"},
				Skipped:   []string{"c"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.HasChanges(); got != tt.expected {
				t.Errorf("HasChanges() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRestoreResultTotalChanges(t *testing.T) {
	tests := []struct {
		name     string
		result   RestoreResult
		expected int
	}{
		{
			name:     "no changes",
			result:   RestoreResult{},
			expected: 0,
		},
		{
			name: "all types",
			result: RestoreResult{
				Added:   []string{"a", "b"},
				Updated: []string{"c"},
				Deleted: []string{"d", "e", "f"},
			},
			expected: 6,
		},
		{
			name: "unchanged and skipped not counted",
			result: RestoreResult{
				Added:     []string{"a"},
				Unchanged: []string{"b", "c"},
				Skipped:   []string{"d"},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.TotalChanges(); got != tt.expected {
				t.Errorf("TotalChanges() = %v, want %v", got, tt.expected)
			}
		})
	}
}
