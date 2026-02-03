package vault

import (
	"context"
	"fmt"
)

// Add writes a new key-value pair to a secret.
// The path format is secret/path/key - parent path is the secret, last segment is the key.
// Creates the secret if it doesn't exist.
// Returns an error if the key already exists (use Update instead).
func (c *Client) Add(ctx context.Context, path, value string) error {
	secretPath, key := ParseWritePath(path)
	if key == "" {
		return fmt.Errorf("path must include a key: %s (e.g., %s/keyname)", path, path)
	}

	// Check if the key already exists
	exists, err := c.KeyExists(ctx, secretPath, key)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("key %q already exists at %s (use 'update' to modify existing keys)", key, secretPath)
	}

	return c.WriteKey(ctx, secretPath, key, value)
}

// Update updates an existing key in a secret.
// The path format is secret/path/key - resolves to find the secret and key.
// Returns an error if the key does not exist.
func (c *Client) Update(ctx context.Context, path, value string) error {
	// Try to resolve the path to find existing secret/key
	resolved, err := c.ResolvePath(ctx, path)
	if err != nil {
		// Path doesn't exist - try parsing as write path
		secretPath, key := ParseWritePath(path)
		if key == "" {
			return fmt.Errorf("key not found at %s", path)
		}
		return fmt.Errorf("key %q not found at %s", key, secretPath)
	}

	if resolved.Key == "" {
		return fmt.Errorf("path %s is a secret, not a key (specify a key to update)", path)
	}

	return c.WriteKey(ctx, resolved.SecretPath, resolved.Key, value)
}

// GetValue retrieves a specific key from a secret.
// The path should be a secret path, and key is the key name within it.
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

// Get retrieves secrets at a path.
// If the path resolves to a key, returns just that key's value.
// If it resolves to a secret, returns all key-value pairs.
// For directories, recursively gets all secrets under it.
func (c *Client) Get(ctx context.Context, path string) (map[string]any, error) {
	// First, try to resolve the path to see if it points to a specific key
	resolved, err := c.ResolvePath(ctx, path)
	if err == nil {
		if resolved.Key != "" {
			// Path resolves to a specific key
			value, err := c.ReadKey(ctx, resolved.SecretPath, resolved.Key)
			if err != nil {
				return nil, err
			}
			return map[string]any{resolved.Key: value}, nil
		}
		// Path is a secret - return all its keys
		data, err := c.ReadSecretRaw(ctx, resolved.SecretPath)
		if err != nil {
			return nil, err
		}
		if data == nil {
			return nil, fmt.Errorf("no secrets found at %s", path)
		}
		return data, nil
	}

	// Path didn't resolve to an existing secret - try as a directory
	result := make(map[string]any)
	if err := c.getRecursive(ctx, path, result); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no secrets found at %s", path)
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
// Also supports reading a specific key if resolved path has one
func (c *Client) readAndValidateSource(ctx context.Context, src string) (map[string]any, *ResolvedPath, error) {
	// Try to resolve the path
	resolved, err := c.ResolvePath(ctx, src)
	if err != nil {
		return nil, nil, fmt.Errorf("source not found: %s", src)
	}

	srcData, err := c.ReadSecretRaw(ctx, resolved.SecretPath)
	if err != nil {
		return nil, nil, err
	}
	if len(srcData) == 0 {
		return nil, nil, fmt.Errorf("source secret does not exist: %s", src)
	}

	// If a specific key was requested, extract just that
	if resolved.Key != "" {
		if val, ok := srcData[resolved.Key]; ok {
			return map[string]any{resolved.Key: val}, resolved, nil
		}
		return nil, nil, fmt.Errorf("key %q not found in %s", resolved.Key, resolved.SecretPath)
	}

	return srcData, resolved, nil
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

// Copy copies a secret or key from src to dst.
// If src is a key path, copies just that key to dst (which should also be a key path).
// If src is a secret path, copies the whole secret.
// Returns an error if the destination already exists.
func (c *Client) Copy(ctx context.Context, src, dst string) error {
	srcData, srcResolved, err := c.readAndValidateSource(ctx, src)
	if err != nil {
		return err
	}

	if srcResolved.Key != "" {
		// Copying a single key - dst should be a key path
		dstSecret, dstKey := ParseWritePath(dst)
		if dstKey == "" {
			// If no key specified in dst, use source key name
			dstKey = srcResolved.Key
			dstSecret = dst
		}

		// Check if destination key exists
		exists, err := c.KeyExists(ctx, dstSecret, dstKey)
		if err != nil {
			// Secret might not exist, that's OK
			exists = false
		}
		if exists {
			return fmt.Errorf("destination key already exists: %s/%s", dstSecret, dstKey)
		}

		return c.WriteKey(ctx, dstSecret, dstKey, fmt.Sprintf("%v", srcData[srcResolved.Key]))
	}

	// Copying a whole secret
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

// Move moves a secret or key from src to dst.
// If src is a key path, moves just that key to dst.
// If src is a secret path, moves the whole secret.
// Returns an error if the destination already exists.
func (c *Client) Move(ctx context.Context, src, dst string) error {
	srcData, srcResolved, err := c.readAndValidateSource(ctx, src)
	if err != nil {
		return err
	}

	if srcResolved.Key != "" {
		// Moving a single key
		dstSecret, dstKey := ParseWritePath(dst)
		if dstKey == "" {
			// If no key specified in dst, use source key name
			dstKey = srcResolved.Key
			dstSecret = dst
		}

		// Check if destination key exists
		exists, err := c.KeyExists(ctx, dstSecret, dstKey)
		if err != nil {
			exists = false
		}
		if exists {
			return fmt.Errorf("destination key already exists: %s/%s", dstSecret, dstKey)
		}

		// Write to destination
		if err := c.WriteKey(ctx, dstSecret, dstKey, fmt.Sprintf("%v", srcData[srcResolved.Key])); err != nil {
			return err
		}

		// Delete from source
		if err := c.DeleteKey(ctx, srcResolved.SecretPath, srcResolved.Key); err != nil {
			// Rollback - delete the destination key
			_ = c.DeleteKey(ctx, dstSecret, dstKey)
			return fmt.Errorf("failed to delete source key: %w", err)
		}

		return nil
	}

	// Moving a whole secret
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

// Import imports secrets from a nested map.
// The YAML is flattened to dot-notation keys and stored in a single secret at basePath.
// For example:
//
//	db:
//	  password: secret123
//	  host: localhost
//
// Becomes a secret at basePath with keys: db.password, db.host
// Returns the number of keys written.
func (c *Client) Import(ctx context.Context, basePath string, data map[string]any) (int, error) {
	// Flatten the nested map to dot-notation keys
	flattened := Flatten(data)

	// Convert all values to strings
	secretData := make(map[string]any)
	for k, v := range flattened {
		secretData[k] = fmt.Sprintf("%v", v)
	}

	if len(secretData) == 0 {
		return 0, fmt.Errorf("no keys to import")
	}

	if err := c.WriteSecret(ctx, basePath, secretData); err != nil {
		return 0, err
	}

	return len(secretData), nil
}

// ImportWithMount imports secrets with an explicit mount point.
// Use this when the mount path contains slashes (e.g., "satellite/slc").
func (c *Client) ImportWithMount(ctx context.Context, mount, basePath string, data map[string]any) (int, error) {
	// For mount-specified imports, use the provided mount in path construction
	fullPath := mount + "/" + basePath
	return c.Import(ctx, fullPath, data)
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
			// Use slash notation for the full path: currentPath/key
			fullPath := currentPath + "/" + key
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
				// Use slash notation for the full path: secretPath/key
				fullPath := secretPath + "/" + key
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
