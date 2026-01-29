package vault

import (
	"context"
	"fmt"
)

// Add writes a new secret value at the given path.
// The value is stored as {"value": value}.
// Returns an error if the secret already exists (use Update instead).
func (c *Client) Add(ctx context.Context, path, value string) error {
	exists, err := c.SecretExists(ctx, path)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("secret already exists at %s (use 'update' to modify existing secrets)", path)
	}

	data := map[string]any{
		"value": value,
	}
	return c.WriteSecret(ctx, path, data)
}

// Update updates an existing secret value at the given path.
// Returns an error if the secret does not exist.
func (c *Client) Update(ctx context.Context, path, value string) error {
	exists, err := c.SecretExists(ctx, path)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("secret not found at %s", path)
	}

	data := map[string]any{
		"value": value,
	}
	return c.WriteSecret(ctx, path, data)
}

// GetValue retrieves a specific key from a secret at the given path.
func (c *Client) GetValue(ctx context.Context, path, key string) (any, error) {
	data, err := c.ReadSecretRaw(ctx, path)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, fmt.Errorf("secret not found at %s", path)
	}

	value, ok := data[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found in secret at %s", key, path)
	}

	return value, nil
}

// Get retrieves all secrets at a path recursively, returning them as a nested map.
func (c *Client) Get(ctx context.Context, path string) (map[string]any, error) {
	result := make(map[string]any)
	if err := c.getRecursive(ctx, path, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) getRecursive(ctx context.Context, vaultPath string, result map[string]any) error {
	dirs, hasSecrets, err := c.ListDirectories(ctx, vaultPath)
	if err != nil {
		return err
	}

	// If this path has secrets, get them
	if hasSecrets {
		secrets, err := c.ListSecrets(ctx, vaultPath)
		if err != nil {
			return err
		}
		for k, v := range secrets {
			result[k] = v
		}
	}

	// If no listing results, try reading directly (leaf secret)
	if !hasSecrets && len(dirs) == 0 {
		data, err := c.ReadSecretRaw(ctx, vaultPath)
		if err != nil {
			return err
		}
		if len(data) > 0 {
			for k, v := range data {
				result[k] = v
			}
			return nil
		}
	}

	// Recurse into subdirectories
	for _, dir := range dirs {
		subPath := vaultPath + "/" + dir
		subResult := make(map[string]any)

		if err := c.getRecursive(ctx, subPath, subResult); err != nil {
			return err
		}

		if len(subResult) > 0 {
			result[dir] = subResult
		}
	}

	return nil
}

// ListEntry represents an entry in a directory listing
type ListEntry struct {
	Name      string
	IsDir     bool
	Version   int
	UpdatedAt string
}

// List returns the contents of a path (directories and secrets).
func (c *Client) List(ctx context.Context, path string) ([]ListEntry, error) {
	dirs, hasSecrets, err := c.ListDirectories(ctx, path)
	if err != nil {
		return nil, err
	}

	var entries []ListEntry
	seen := make(map[string]bool)

	// Add directories
	for _, dir := range dirs {
		entries = append(entries, ListEntry{
			Name:  dir,
			IsDir: true,
		})
		seen[dir] = true
	}

	// Add secrets
	if hasSecrets {
		paths, err := c.ListSecretPaths(ctx, path)
		if err != nil {
			return nil, err
		}
		// Only get immediate children, not nested paths
		for _, p := range paths {
			// Get first component only
			for i, ch := range p {
				if ch == '/' {
					p = p[:i]
					break
				}
			}
			// Skip if already added as directory
			if !seen[p] {
				entries = append(entries, ListEntry{
					Name:  p,
					IsDir: false,
				})
				seen[p] = true
			}
		}
	}

	return entries, nil
}

// ListWithMetadata returns the contents of a path with metadata for secrets.
func (c *Client) ListWithMetadata(ctx context.Context, path string) ([]ListEntry, error) {
	entries, err := c.List(ctx, path)
	if err != nil {
		return nil, err
	}

	// Fetch metadata for secrets
	for i := range entries {
		if !entries[i].IsDir {
			secretPath := path + "/" + entries[i].Name
			metadata, err := c.GetMetadata(ctx, secretPath)
			if err == nil && metadata != nil {
				entries[i].Version = metadata.CurrentVersion
				if !metadata.UpdatedTime.IsZero() {
					entries[i].UpdatedAt = metadata.UpdatedTime.Format("2006-01-02 15:04:05")
				}
			}
		}
	}

	return entries, nil
}

// readAndValidateSource reads source data and validates it exists
func (c *Client) readAndValidateSource(ctx context.Context, src string) (map[string]any, error) {
	srcData, err := c.ReadSecretRaw(ctx, src)
	if err != nil {
		return nil, err
	}
	if len(srcData) == 0 {
		return nil, fmt.Errorf("source secret does not exist: %s", src)
	}
	return srcData, nil
}

// checkDestinationNotExists validates that destination doesn't exist
func (c *Client) checkDestinationNotExists(ctx context.Context, dst string) error {
	exists, err := c.SecretExists(ctx, dst)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("destination already exists: %s", dst)
	}
	return nil
}

// checkDestinationsNotExist validates that none of the destination paths exist
func (c *Client) checkDestinationsNotExist(ctx context.Context, dst string, relPaths []string) error {
	for _, relPath := range relPaths {
		if err := c.checkDestinationNotExists(ctx, dst+"/"+relPath); err != nil {
			return err
		}
	}
	return nil
}

// copySecrets copies secrets from src to dst for the given relative paths
func (c *Client) copySecrets(ctx context.Context, src, dst string, relPaths []string) error {
	for _, relPath := range relPaths {
		srcPath := src + "/" + relPath
		dstPath := dst + "/" + relPath

		srcData, err := c.ReadSecretRaw(ctx, srcPath)
		if err != nil {
			return err
		}

		if err := c.WriteSecret(ctx, dstPath, srcData); err != nil {
			return fmt.Errorf("failed to write %s: %w", dstPath, err)
		}
	}
	return nil
}

// Copy copies a single secret from src to dst.
// Returns an error if the destination already exists.
func (c *Client) Copy(ctx context.Context, src, dst string) error {
	srcData, err := c.readAndValidateSource(ctx, src)
	if err != nil {
		return err
	}

	if err := c.checkDestinationNotExists(ctx, dst); err != nil {
		return err
	}

	return c.WriteSecret(ctx, dst, srcData)
}

// CopyRecursive copies all secrets under src to dst.
// Returns the number of secrets copied.
func (c *Client) CopyRecursive(ctx context.Context, src, dst string) (int, error) {
	secretPaths, err := c.ListSecretPaths(ctx, src)
	if err != nil {
		return 0, err
	}

	if len(secretPaths) == 0 {
		// Try as a single secret
		if err := c.Copy(ctx, src, dst); err != nil {
			return 0, err
		}
		return 1, nil
	}

	if err := c.checkDestinationsNotExist(ctx, dst, secretPaths); err != nil {
		return 0, err
	}

	if err := c.copySecrets(ctx, src, dst, secretPaths); err != nil {
		return 0, err
	}

	return len(secretPaths), nil
}

// Move moves a single secret from src to dst.
// Returns an error if the destination already exists.
func (c *Client) Move(ctx context.Context, src, dst string) error {
	srcData, err := c.readAndValidateSource(ctx, src)
	if err != nil {
		return err
	}

	if err := c.checkDestinationNotExists(ctx, dst); err != nil {
		return err
	}

	if err := c.WriteSecret(ctx, dst, srcData); err != nil {
		return err
	}

	if err := c.DeleteSecret(ctx, src); err != nil {
		// Try to clean up the destination we just created
		if rollbackErr := c.DeleteSecret(ctx, dst); rollbackErr != nil {
			return fmt.Errorf("failed to delete source (%w) and rollback failed: %v", err, rollbackErr)
		}
		return fmt.Errorf("failed to delete source after copy: %w", err)
	}

	return nil
}

// MoveRecursive moves all secrets under src to dst.
// Returns the number of secrets moved.
func (c *Client) MoveRecursive(ctx context.Context, src, dst string) (int, error) {
	secretPaths, err := c.ListSecretPaths(ctx, src)
	if err != nil {
		return 0, err
	}

	if len(secretPaths) == 0 {
		return 0, fmt.Errorf("no secrets found under: %s", src)
	}

	if err := c.checkDestinationsNotExist(ctx, dst, secretPaths); err != nil {
		return 0, err
	}

	// Copy all secrets first (with rollback support)
	var copiedPaths []string
	for _, relPath := range secretPaths {
		srcPath := src + "/" + relPath
		dstPath := dst + "/" + relPath

		srcData, err := c.ReadSecretRaw(ctx, srcPath)
		if err != nil {
			return 0, err
		}

		if err := c.WriteSecret(ctx, dstPath, srcData); err != nil {
			// Rollback: delete already copied secrets
			var rollbackErrors []string
			for _, copied := range copiedPaths {
				if rollbackErr := c.DeleteSecret(ctx, copied); rollbackErr != nil {
					rollbackErrors = append(rollbackErrors, fmt.Sprintf("%s: %v", copied, rollbackErr))
				}
			}
			if len(rollbackErrors) > 0 {
				return 0, fmt.Errorf("failed to write %s (%w) and rollback failed for: %v", dstPath, err, rollbackErrors)
			}
			return 0, fmt.Errorf("failed to write %s: %w", dstPath, err)
		}
		copiedPaths = append(copiedPaths, dstPath)
	}

	// Delete source secrets
	// Note: If deletion fails partway, copies at destination will remain.
	// This is intentional - it's safer to have duplicates than data loss.
	var deleteErrors []string
	deletedCount := 0
	for _, relPath := range secretPaths {
		srcPath := src + "/" + relPath
		if err := c.DeleteSecret(ctx, srcPath); err != nil {
			deleteErrors = append(deleteErrors, fmt.Sprintf("%s: %v", srcPath, err))
		} else {
			deletedCount++
		}
	}

	if len(deleteErrors) > 0 {
		return deletedCount, fmt.Errorf("move partially completed: %d/%d sources deleted, failed to delete: %v",
			deletedCount, len(secretPaths), deleteErrors)
	}

	return len(secretPaths), nil
}

// DeleteRecursiveResult contains information about a recursive delete operation
type DeleteRecursiveResult struct {
	Deleted []string
	Count   int
}

// DeleteRecursive deletes all secrets under the given path.
func (c *Client) DeleteRecursive(ctx context.Context, path string) (*DeleteRecursiveResult, error) {
	result := &DeleteRecursiveResult{}
	if err := c.deleteRecursive(ctx, path, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) deleteRecursive(ctx context.Context, path string, result *DeleteRecursiveResult) error {
	dirs, hasSecrets, err := c.ListDirectories(ctx, path)
	if err != nil {
		return err
	}

	// Delete secrets at this level
	if hasSecrets {
		paths, err := c.ListSecretPaths(ctx, path)
		if err != nil {
			return err
		}

		for _, p := range paths {
			fullPath := path + "/" + p
			if err := c.DeleteSecret(ctx, fullPath); err != nil {
				return err
			}
			result.Deleted = append(result.Deleted, fullPath)
			result.Count++
		}
	}

	// Recurse into subdirectories
	for _, dir := range dirs {
		subPath := path + "/" + dir
		if err := c.deleteRecursive(ctx, subPath, result); err != nil {
			return err
		}
	}

	return nil
}

// Export retrieves all secrets at a path for export.
// This is an alias for ListSecrets which already returns the correct format.
func (c *Client) Export(ctx context.Context, path string) (map[string]any, error) {
	return c.ListSecrets(ctx, path)
}

// Import imports secrets from a nested map, flattening and writing each value.
// Returns the number of secrets written.
func (c *Client) Import(ctx context.Context, basePath string, data map[string]any) (int, error) {
	flattened := Flatten(data)
	return c.WriteSecrets(ctx, basePath, flattened)
}

// ImportWithMount imports secrets with an explicit mount point.
// Use this when the mount path contains slashes (e.g., "satellite/slc").
func (c *Client) ImportWithMount(ctx context.Context, mount, basePath string, data map[string]any) (int, error) {
	flattened := Flatten(data)
	return c.WriteSecretsWithMount(ctx, mount, basePath, flattened)
}

// DuplicateGroup represents a group of paths that share the same value
type DuplicateGroup struct {
	Paths []string
}

// FindDuplicates finds secrets with duplicate values under the given path.
func (c *Client) FindDuplicates(ctx context.Context, path string) ([]DuplicateGroup, error) {
	// Map of value hash -> list of paths with that value
	valueMap := make(map[string][]string)

	if err := c.collectValues(ctx, path, "", valueMap); err != nil {
		return nil, err
	}

	// Find duplicates
	var duplicates []DuplicateGroup
	for _, paths := range valueMap {
		if len(paths) > 1 {
			duplicates = append(duplicates, DuplicateGroup{Paths: paths})
		}
	}

	return duplicates, nil
}

func (c *Client) collectValues(ctx context.Context, basePath, prefix string, valueMap map[string][]string) error {
	currentPath := basePath
	if prefix != "" {
		currentPath = basePath + "/" + prefix
	}

	// Check if this is a secret we can read directly
	data, err := c.ReadSecretRaw(ctx, currentPath)
	if err != nil {
		return err
	}

	if len(data) > 0 {
		// Process each key in the secret
		for key, value := range data {
			fullPath := currentPath + "." + key
			hash := hashValue(value)
			valueMap[hash] = append(valueMap[hash], fullPath)
		}
	}

	// Check for subdirectories/secrets
	dirs, hasSecrets, err := c.ListDirectories(ctx, currentPath)
	if err != nil {
		return err
	}

	if hasSecrets {
		paths, err := c.ListSecretPaths(ctx, currentPath)
		if err != nil {
			return err
		}

		for _, p := range paths {
			secretPath := currentPath + "/" + p
			secretData, err := c.ReadSecretRaw(ctx, secretPath)
			if err != nil {
				return err
			}

			for key, value := range secretData {
				fullPath := secretPath + "." + key
				hash := hashValue(value)
				valueMap[hash] = append(valueMap[hash], fullPath)
			}
		}
	}

	// Recurse into subdirectories
	for _, dir := range dirs {
		subPath := dir
		if prefix != "" {
			subPath = prefix + "/" + dir
		}
		if err := c.collectValues(ctx, basePath, subPath, valueMap); err != nil {
			return err
		}
	}

	return nil
}
