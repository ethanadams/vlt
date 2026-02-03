package vault

import (
	"context"
	"fmt"
	"time"
)

// Snapshot represents a point-in-time backup of secrets
type Snapshot struct {
	// Metadata
	Path      string    `yaml:"path"`
	CreatedAt time.Time `yaml:"created_at"`

	// Secrets maps relative paths to their data
	Secrets map[string]SnapshotSecret `yaml:"secrets"`
}

// SnapshotSecret represents a single secret in a snapshot
// With grouped storage, Data contains all key-value pairs in the secret
type SnapshotSecret struct {
	Data    map[string]any `yaml:"data"`    // All key-value pairs in the secret
	Version int            `yaml:"version"`
	Updated time.Time      `yaml:"updated"`
}

// RestoreOptions configures how a restore operation behaves
type RestoreOptions struct {
	DryRun       bool // Preview changes without applying
	Verify       bool // Only restore if versions match
	DeleteExtra  bool // Delete secrets not in snapshot (default true)
}

// RestoreResult contains the results of a restore operation
type RestoreResult struct {
	Added    []string // Secrets that were added
	Updated  []string // Secrets that were updated
	Deleted  []string // Secrets that were deleted
	Unchanged []string // Secrets that were unchanged
	Skipped  []string // Secrets skipped due to verification failure
}

// CreateSnapshot creates a snapshot of all secrets under a path
func (c *Client) CreateSnapshot(ctx context.Context, path string) (*Snapshot, error) {
	// Get all secret paths
	secretPaths, err := c.ListSecretPaths(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	if len(secretPaths) == 0 {
		return nil, fmt.Errorf("no secrets found at %s", path)
	}

	snapshot := &Snapshot{
		Path:      path,
		CreatedAt: time.Now(),
		Secrets:   make(map[string]SnapshotSecret),
	}

	for _, relPath := range secretPaths {
		fullPath := path + "/" + relPath

		// Read the secret data (all key-value pairs)
		data, err := c.ReadSecretRaw(ctx, fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read secret %s: %w", relPath, err)
		}

		// Get metadata for version info
		metadata, err := c.GetMetadata(ctx, fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get metadata for %s: %w", relPath, err)
		}

		snapshot.Secrets[relPath] = SnapshotSecret{
			Data:    data,
			Version: metadata.CurrentVersion,
			Updated: metadata.UpdatedTime,
		}
	}

	return snapshot, nil
}

// RestoreSnapshot restores secrets from a snapshot
func (c *Client) RestoreSnapshot(ctx context.Context, snapshot *Snapshot, targetPath string, opts RestoreOptions) (*RestoreResult, error) {
	result := &RestoreResult{
		Added:     make([]string, 0),
		Updated:   make([]string, 0),
		Deleted:   make([]string, 0),
		Unchanged: make([]string, 0),
		Skipped:   make([]string, 0),
	}

	// Get current secrets at target path
	currentPaths, err := c.ListSecretPaths(ctx, targetPath)
	if err != nil {
		// Path might not exist yet, that's OK
		currentPaths = []string{}
	}

	currentSet := make(map[string]bool)
	for _, p := range currentPaths {
		currentSet[p] = true
	}

	// Process secrets from snapshot
	for relPath, snapshotSecret := range snapshot.Secrets {
		fullPath := targetPath + "/" + relPath

		exists := currentSet[relPath]
		delete(currentSet, relPath) // Remove from set to track what's left

		if opts.Verify && exists {
			// Check if current version matches snapshot version
			metadata, err := c.GetMetadata(ctx, fullPath)
			if err == nil && metadata.CurrentVersion != snapshotSecret.Version {
				result.Skipped = append(result.Skipped, relPath)
				continue
			}
		}

		// Check if secret needs to be updated
		if exists {
			// Read current data to compare
			currentData, err := c.ReadSecretRaw(ctx, fullPath)
			if err == nil {
				// Compare all key-value pairs
				if mapsEqual(currentData, snapshotSecret.Data) {
					result.Unchanged = append(result.Unchanged, relPath)
					continue
				}
			}
			result.Updated = append(result.Updated, relPath)
		} else {
			result.Added = append(result.Added, relPath)
		}

		if !opts.DryRun {
			// Write the secret with all its key-value pairs
			if err := c.WriteSecret(ctx, fullPath, snapshotSecret.Data); err != nil {
				return nil, fmt.Errorf("failed to write secret %s: %w", relPath, err)
			}
		}
	}

	// Handle secrets that exist in Vault but not in snapshot (delete them)
	if opts.DeleteExtra {
		for relPath := range currentSet {
			result.Deleted = append(result.Deleted, relPath)
			if !opts.DryRun {
				fullPath := targetPath + "/" + relPath
				if err := c.DeleteSecret(ctx, fullPath); err != nil {
					return nil, fmt.Errorf("failed to delete secret %s: %w", relPath, err)
				}
			}
		}
	}

	return result, nil
}

// HasChanges returns true if the restore would make any changes
func (r *RestoreResult) HasChanges() bool {
	return len(r.Added) > 0 || len(r.Updated) > 0 || len(r.Deleted) > 0
}

// TotalChanges returns the total number of changes
func (r *RestoreResult) TotalChanges() int {
	return len(r.Added) + len(r.Updated) + len(r.Deleted)
}

// mapsEqual compares two maps for equality
func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", va) != fmt.Sprintf("%v", vb) {
			return false
		}
	}
	return true
}
