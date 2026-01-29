package vault

import (
	"reflect"
	"testing"
)

func TestFlatten(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: map[string]any{},
		},
		{
			name: "flat map unchanged",
			input: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
			expected: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "nested map flattened",
			input: map[string]any{
				"database": map[string]any{
					"host":     "localhost",
					"password": "secret",
				},
			},
			expected: map[string]any{
				"database.host":     "localhost",
				"database.password": "secret",
			},
		},
		{
			name: "deeply nested",
			input: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": "deep",
					},
				},
			},
			expected: map[string]any{
				"a.b.c": "deep",
			},
		},
		{
			name: "mixed flat and nested",
			input: map[string]any{
				"flat": "value",
				"nested": map[string]any{
					"key": "nested-value",
				},
			},
			expected: map[string]any{
				"flat":       "value",
				"nested.key": "nested-value",
			},
		},
		{
			name: "numeric values",
			input: map[string]any{
				"count": 42,
				"nested": map[string]any{
					"port": 8080,
				},
			},
			expected: map[string]any{
				"count":       42,
				"nested.port": 8080,
			},
		},
		{
			name:     "nil input",
			input:    nil,
			expected: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Flatten(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Flatten() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFlattenPreservesOrder(t *testing.T) {
	// Flatten should be deterministic
	input := map[string]any{
		"z": "last",
		"a": "first",
		"m": map[string]any{
			"x": "nested",
		},
	}

	result1 := Flatten(input)
	result2 := Flatten(input)

	if !reflect.DeepEqual(result1, result2) {
		t.Errorf("Flatten() not deterministic: %v != %v", result1, result2)
	}
}
