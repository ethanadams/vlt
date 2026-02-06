package vault

import (
	"context"
	"path"
	"sort"
)

// FindResult represents a key that matched a find pattern
type FindResult struct {
	SecretPath string // Full path to the secret
	Key        string // Key name within the secret
}

// FullPath returns the combined secret path and key
func (r FindResult) FullPath() string {
	return r.SecretPath + "/" + r.Key
}

// FindKeys searches for keys matching a glob pattern at the given path.
// Without recursive, only searches secrets at the immediate level.
// With recursive, searches all secrets under the path.
// Pattern supports glob syntax (*, ?, [...]).
func (c *Client) FindKeys(ctx context.Context, basePath, pattern string, recursive bool) ([]FindResult, error) {
	var results []FindResult

	if recursive {
		if err := c.findKeysRecursive(ctx, basePath, pattern, &results); err != nil {
			return nil, err
		}
	} else {
		if err := c.findKeysAtLevel(ctx, basePath, pattern, &results); err != nil {
			return nil, err
		}
	}

	// Sort results by full path for consistent output
	sort.Slice(results, func(i, j int) bool {
		return results[i].FullPath() < results[j].FullPath()
	})

	return results, nil
}

// findKeysAtLevel searches keys in secrets at the immediate level of basePath
func (c *Client) findKeysAtLevel(ctx context.Context, basePath, pattern string, results *[]FindResult) error {
	// First check if basePath itself is a secret
	data, err := c.ReadSecretRaw(ctx, basePath)
	if err != nil {
		return err
	}

	if len(data) > 0 {
		matchKeys(basePath, data, pattern, results)
		return nil
	}

	// List entries at this level
	dirs, hasSecrets, err := c.ListDirectories(ctx, basePath)
	if err != nil {
		return err
	}

	_ = dirs // We only care about secrets at this level, not subdirectories

	if hasSecrets {
		paths, err := c.ListSecretPaths(ctx, basePath)
		if err != nil {
			return err
		}

		for _, p := range paths {
			// Only immediate children (no slash in path)
			isImmediate := true
			for _, ch := range p {
				if ch == '/' {
					isImmediate = false
					break
				}
			}
			if !isImmediate {
				continue
			}

			secretPath := basePath + "/" + p
			secretData, err := c.ReadSecretRaw(ctx, secretPath)
			if err != nil {
				return err
			}
			matchKeys(secretPath, secretData, pattern, results)
		}
	}

	return nil
}

// findKeysRecursive searches keys in all secrets under basePath
func (c *Client) findKeysRecursive(ctx context.Context, basePath, pattern string, results *[]FindResult) error {
	// Check if basePath itself is a secret
	data, err := c.ReadSecretRaw(ctx, basePath)
	if err != nil {
		return err
	}

	if len(data) > 0 {
		matchKeys(basePath, data, pattern, results)
		return nil
	}

	// ListSecretPaths is already recursive — returns all secret paths under basePath
	paths, err := c.ListSecretPaths(ctx, basePath)
	if err != nil {
		return err
	}

	for _, p := range paths {
		secretPath := basePath + "/" + p
		secretData, err := c.ReadSecretRaw(ctx, secretPath)
		if err != nil {
			return err
		}
		matchKeys(secretPath, secretData, pattern, results)
	}

	return nil
}

// matchKeys checks each key in the data against the pattern and appends matches
func matchKeys(secretPath string, data map[string]any, pattern string, results *[]FindResult) {
	for key := range data {
		matched, err := path.Match(pattern, key)
		if err != nil {
			// Invalid pattern, skip (shouldn't happen if validated upstream)
			continue
		}
		if matched {
			*results = append(*results, FindResult{
				SecretPath: secretPath,
				Key:        key,
			})
		}
	}
}
