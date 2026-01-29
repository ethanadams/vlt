package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// DiffResult holds the comparison between two secret maps
type DiffResult struct {
	OnlyInFirst  []DiffEntry
	OnlyInSecond []DiffEntry
	Changed      []ChangedEntry
	Unchanged    int
}

// DiffEntry represents a key that exists only in one source
type DiffEntry struct {
	Key   string
	Value string
}

// ChangedEntry represents a key with different values in each source
type ChangedEntry struct {
	Key         string
	FirstLen    int
	SecondLen   int
	FirstValue  string
	SecondValue string
}

// HasDifferences returns true if there are any differences
func (d *DiffResult) HasDifferences() bool {
	return len(d.OnlyInFirst) > 0 || len(d.OnlyInSecond) > 0 || len(d.Changed) > 0
}

// CompareSecrets compares two flattened secret maps and returns the differences
func CompareSecrets(secrets1, secrets2 map[string]any) *DiffResult {
	result := &DiffResult{}

	// Find keys only in first, only in second, and changed
	for key, val1 := range secrets1 {
		val1Str := fmt.Sprintf("%v", val1)
		if val2, exists := secrets2[key]; exists {
			val2Str := fmt.Sprintf("%v", val2)
			// Key exists in both - compare values
			if hashValue(val1) != hashValue(val2) {
				result.Changed = append(result.Changed, ChangedEntry{
					Key:         key,
					FirstLen:    len(val1Str),
					SecondLen:   len(val2Str),
					FirstValue:  val1Str,
					SecondValue: val2Str,
				})
			} else {
				result.Unchanged++
			}
		} else {
			result.OnlyInFirst = append(result.OnlyInFirst, DiffEntry{Key: key, Value: val1Str})
		}
	}

	for key, val2 := range secrets2 {
		if _, exists := secrets1[key]; !exists {
			result.OnlyInSecond = append(result.OnlyInSecond, DiffEntry{Key: key, Value: fmt.Sprintf("%v", val2)})
		}
	}

	// Sort for consistent output
	sort.Slice(result.OnlyInFirst, func(i, j int) bool {
		return result.OnlyInFirst[i].Key < result.OnlyInFirst[j].Key
	})
	sort.Slice(result.OnlyInSecond, func(i, j int) bool {
		return result.OnlyInSecond[i].Key < result.OnlyInSecond[j].Key
	})
	sort.Slice(result.Changed, func(i, j int) bool {
		return result.Changed[i].Key < result.Changed[j].Key
	})

	return result
}

// VersionSpec represents a version specification parsed from a path
type VersionSpec struct {
	Version      int  // Positive version number (e.g., @3)
	ChangesAgo   int  // Negative offset for changes ago (e.g., @-2 means 2 changes ago)
	IsPrev       bool // @prev alias
	IsChangesAgo bool // True if using @-N syntax
}

// HasVersion returns true if any version specifier was provided
func (v VersionSpec) HasVersion() bool {
	return v.Version > 0 || v.IsPrev || v.IsChangesAgo
}

// ParseVersionedPath extracts path and version from "path@version" format
// Supports:
//   - @N for specific version (single secrets only)
//   - @prev for previous version
//   - @-N for N changes ago (directories only, based on change timeline)
func ParseVersionedPath(path string) (string, VersionSpec) {
	spec := VersionSpec{}

	if idx := strings.LastIndex(path, "@"); idx != -1 {
		versionStr := path[idx+1:]
		basePath := path[:idx]

		// Check for @prev alias
		if versionStr == "prev" || versionStr == "previous" {
			spec.IsPrev = true
			return basePath, spec
		}

		// Parse numeric version (positive or negative)
		if version, err := strconv.Atoi(versionStr); err == nil {
			if version < 0 {
				// @-N means N changes ago
				spec.ChangesAgo = -version
				spec.IsChangesAgo = true
				return basePath, spec
			} else if version > 0 {
				spec.Version = version
				return basePath, spec
			}
		}
	}
	return path, spec
}

// VersionChange represents a change between two versions
type VersionChange struct {
	Key       string
	Type      ChangeType // Added, Changed, Deleted
	OldValue  string     // Empty for Added
	NewValue  string     // Empty for Deleted
	OldLength int
	NewLength int
}

// ChangeType indicates the type of change
type ChangeType int

const (
	ChangeAdded ChangeType = iota
	ChangeModified
	ChangeDeleted
)

// CompareVersions compares two versions of a secret and returns the changes
func (c *Client) CompareVersions(ctx context.Context, path string, oldVersion, newVersion int) ([]VersionChange, error) {
	oldData, err := c.ReadSecretVersion(ctx, path, oldVersion)
	if err != nil {
		return nil, err
	}

	newData, err := c.ReadSecretVersion(ctx, path, newVersion)
	if err != nil {
		return nil, err
	}

	var changes []VersionChange

	// Find added and changed keys
	for key, newVal := range newData {
		newValStr := fmt.Sprintf("%v", newVal)
		oldVal, exists := oldData[key]
		if !exists {
			changes = append(changes, VersionChange{
				Key:       key,
				Type:      ChangeAdded,
				NewValue:  newValStr,
				NewLength: len(newValStr),
			})
		} else {
			oldValStr := fmt.Sprintf("%v", oldVal)
			if oldValStr != newValStr {
				changes = append(changes, VersionChange{
					Key:       key,
					Type:      ChangeModified,
					OldValue:  oldValStr,
					NewValue:  newValStr,
					OldLength: len(oldValStr),
					NewLength: len(newValStr),
				})
			}
		}
	}

	// Find deleted keys
	for key, oldVal := range oldData {
		if _, exists := newData[key]; !exists {
			oldValStr := fmt.Sprintf("%v", oldVal)
			changes = append(changes, VersionChange{
				Key:       key,
				Type:      ChangeDeleted,
				OldValue:  oldValStr,
				OldLength: len(oldValStr),
			})
		}
	}

	// Sort for consistent output
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Key < changes[j].Key
	})

	return changes, nil
}

// hashValue creates a hash of a value for comparison
func hashValue(value any) string {
	str := fmt.Sprintf("%v", value)
	hash := sha256.Sum256([]byte(str))
	return hex.EncodeToString(hash[:])
}

// FlattenAndExtractValues flattens a nested map and extracts .value fields
// When forDirectory is true, strips standalone "value" key for simple secrets
func FlattenAndExtractValues(data map[string]any, forDirectory bool) map[string]any {
	flattened := Flatten(data)
	result := make(map[string]any)

	for k, v := range flattened {
		key := k
		if forDirectory && k == "value" {
			// Single value secret in directory context - use empty key
			key = ""
		} else if strings.HasSuffix(k, ".value") {
			// Nested secret - strip .value suffix
			key = strings.TrimSuffix(k, ".value")
		}
		result[key] = v
	}

	return result
}
