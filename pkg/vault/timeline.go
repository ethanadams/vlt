package vault

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// TimelineEntry represents a single change in the version timeline
type TimelineEntry struct {
	Time       time.Time
	SecretPath string // Relative path within the directory
	FullPath   string // Full path to the secret
	Version    int
	IsCreation bool // True if this is version 1
}

// GetTimeline returns a chronological timeline of all changes under a path
// Sorted by time descending (newest first)
func (c *Client) GetTimeline(ctx context.Context, path string) ([]TimelineEntry, error) {
	paths, err := c.ListSecretPaths(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets under %s: %w", path, err)
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("no secrets found at %s", path)
	}

	var timeline []TimelineEntry

	for _, secretPath := range paths {
		fullPath := path + "/" + secretPath
		versions, err := c.GetVersionHistory(ctx, fullPath)
		if err != nil {
			continue
		}

		for _, v := range versions {
			timeline = append(timeline, TimelineEntry{
				Time:       v.CreatedTime,
				SecretPath: secretPath,
				FullPath:   fullPath,
				Version:    v.Version,
				IsCreation: v.Version == 1,
			})
		}
	}

	if len(timeline) == 0 {
		return nil, fmt.Errorf("no version history found at %s", path)
	}

	// Sort by time descending (newest first)
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].Time.After(timeline[j].Time)
	})

	return timeline, nil
}

// GetPrevVersions retrieves the previous version of each secret under a path
// Returns a flattened map suitable for comparison
func (c *Client) GetPrevVersions(ctx context.Context, basePath string) (map[string]any, error) {
	secretPaths, err := c.ListSecretPaths(ctx, basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets under %s: %w", basePath, err)
	}

	result := make(map[string]any)
	skipped := 0

	for _, relPath := range secretPaths {
		fullPath := basePath + "/" + relPath

		// Get metadata to find previous version
		metadata, err := c.GetMetadata(ctx, fullPath)
		if err != nil || metadata == nil {
			skipped++
			continue
		}
		if metadata.CurrentVersion <= 1 {
			skipped++
			continue
		}

		prevVersion := metadata.CurrentVersion - 1
		secrets, err := c.ReadSecretVersion(ctx, fullPath, prevVersion)
		if err != nil {
			skipped++
			continue
		}

		// Flatten and extract values
		flattened := FlattenAndExtractValues(secrets, true)
		for k, v := range flattened {
			if k == "" {
				result[relPath] = v
			} else {
				result[relPath+"."+k] = v
			}
		}
	}

	if len(result) == 0 {
		if skipped > 0 {
			return nil, fmt.Errorf("no secrets under %s have a previous version (all are at version 1)", basePath)
		}
		return nil, fmt.Errorf("no secrets found under %s", basePath)
	}

	return result, nil
}

// GetStateAtChangesAgo retrieves the state of a directory N changes ago
// It builds a timeline of all version changes across all secrets, then
// computes what version each secret was at N changes ago
func (c *Client) GetStateAtChangesAgo(ctx context.Context, basePath string, changesAgo int) (map[string]any, error) {
	secretPaths, err := c.ListSecretPaths(ctx, basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets under %s: %w", basePath, err)
	}

	if len(secretPaths) == 0 {
		return nil, fmt.Errorf("no secrets found under %s", basePath)
	}

	// Build timeline of all changes (version > 1 only, since v1 is creation not change)
	type changeEvent struct {
		secretPath  string
		version     int
		createdTime time.Time
	}

	var allChanges []changeEvent
	secretCurrentVersions := make(map[string]int)

	for _, relPath := range secretPaths {
		fullPath := basePath + "/" + relPath
		history, err := c.GetVersionHistory(ctx, fullPath)
		if err != nil || len(history) == 0 {
			continue
		}

		secretCurrentVersions[relPath] = history[0].Version

		for _, v := range history {
			if v.Version > 1 {
				allChanges = append(allChanges, changeEvent{
					secretPath:  relPath,
					version:     v.Version,
					createdTime: v.CreatedTime,
				})
			}
		}
	}

	if len(allChanges) == 0 {
		return nil, fmt.Errorf("no changes found under %s (all secrets are at version 1)", basePath)
	}

	// Sort by time descending (most recent first)
	sort.Slice(allChanges, func(i, j int) bool {
		return allChanges[i].createdTime.After(allChanges[j].createdTime)
	})

	if changesAgo > len(allChanges) {
		return nil, fmt.Errorf("only %d changes exist under %s, cannot go back %d changes", len(allChanges), basePath, changesAgo)
	}

	// Compute the version of each secret N changes ago
	// Start with current versions and "undo" the last N changes
	secretVersionsAtPoint := make(map[string]int)
	for path, ver := range secretCurrentVersions {
		secretVersionsAtPoint[path] = ver
	}

	for i := 0; i < changesAgo && i < len(allChanges); i++ {
		change := allChanges[i]
		if currentVer, ok := secretVersionsAtPoint[change.secretPath]; ok && currentVer == change.version {
			secretVersionsAtPoint[change.secretPath] = change.version - 1
		}
	}

	// Read each secret at the computed version
	result := make(map[string]any)
	for relPath, version := range secretVersionsAtPoint {
		if version < 1 {
			continue
		}

		fullPath := basePath + "/" + relPath
		secrets, err := c.ReadSecretVersion(ctx, fullPath, version)
		if err != nil || secrets == nil {
			continue
		}

		flattened := FlattenAndExtractValues(secrets, true)
		for k, v := range flattened {
			if k == "" {
				result[relPath] = v
			} else {
				result[relPath+"."+k] = v
			}
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no secrets found at %d changes ago", changesAgo)
	}

	return result, nil
}

// GetSecretAtVersion reads a single secret at a specific version
// If isPrev is true, reads the previous version
func (c *Client) GetSecretAtVersion(ctx context.Context, path string, version int, isPrev bool) (map[string]any, error) {
	if isPrev {
		metadata, err := c.GetMetadata(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("failed to get metadata for %s: %w", path, err)
		}
		if metadata == nil {
			return nil, fmt.Errorf("secret not found at %s", path)
		}
		if metadata.CurrentVersion <= 1 {
			return nil, fmt.Errorf("no previous version exists for %s (current version is %d)", path, metadata.CurrentVersion)
		}
		version = metadata.CurrentVersion - 1
	}

	secrets, err := c.ReadSecretVersion(ctx, path, version)
	if err != nil {
		return nil, err
	}
	if secrets == nil {
		return nil, fmt.Errorf("version %d not found at %s", version, path)
	}

	return secrets, nil
}
