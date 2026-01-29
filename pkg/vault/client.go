package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/hashicorp/vault/api"
)

type Client struct {
	client     *api.Client
	mountCache []string // cached KV v2 mounts, sorted by length descending
}

func NewClient(cfg *config.Config) (*Client, error) {
	vaultCfg := api.DefaultConfig()
	vaultCfg.Address = cfg.VaultAddr

	client, err := api.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	client.SetToken(cfg.VaultToken)

	return &Client{client: client}, nil
}

// ListSecrets recursively lists all secrets under a path and returns them as a nested map
func (c *Client) ListSecrets(ctx context.Context, path string) (map[string]any, error) {
	// Determine the mount and secret path
	mount, secretPath, _ := c.ResolveMountPath(ctx, path)

	secrets, err := c.listRecursive(ctx, mount, secretPath)
	if err != nil {
		return nil, err
	}

	// Transform dot-notation keys into nested structure
	return expandSecrets(secrets), nil
}

// expandSecrets transforms a flat map with dot-notation keys into a nested map
// and extracts the "value" field from secret data
func expandSecrets(secrets map[string]any) map[string]any {
	result := make(map[string]any)

	for key, val := range secrets {
		switch v := val.(type) {
		case map[string]any:
			// Check if this is a secret with a "value" field
			if value, ok := v["value"]; ok && len(v) == 1 {
				// Single "value" field - extract it and expand the key
				setNestedValue(result, key, value)
			} else {
				// Nested directory - recurse
				expanded := expandSecrets(v)
				// Merge expanded values with dot-notation expansion
				for k, ev := range expanded {
					setNestedValue(result, key+"."+k, ev)
				}
			}
		default:
			setNestedValue(result, key, v)
		}
	}

	return result
}

// setNestedValue sets a value in a nested map using dot-notation key
// e.g., setNestedValue(m, "admin.oauth2.clientID", "abc") creates m["admin"]["oauth2"]["clientID"] = "abc"
func setNestedValue(m map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	current := m

	for i, part := range parts {
		if i == len(parts)-1 {
			// Last part - set the value
			current[part] = value
		} else {
			// Intermediate part - ensure nested map exists
			if _, ok := current[part]; !ok {
				current[part] = make(map[string]any)
			}
			if nested, ok := current[part].(map[string]any); ok {
				current = nested
			} else {
				// Conflict - existing value is not a map, overwrite
				newMap := make(map[string]any)
				current[part] = newMap
				current = newMap
			}
		}
	}
}

func (c *Client) listRecursive(ctx context.Context, mount, path string) (map[string]any, error) {
	result := make(map[string]any)

	secret, err := c.client.Logical().ListWithContext(ctx, fmt.Sprintf("%s/metadata/%s", mount, ensureTrailingSlash(path)))
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets at %s: %w", path, err)
	}

	if secret == nil || secret.Data == nil {
		// No keys at this path, try to read it as a secret
		data, err := c.readSecret(ctx, mount, path)
		if err != nil {
			return nil, err
		}
		return data, nil
	}

	keys, ok := secret.Data["keys"].([]any)
	if !ok {
		return result, nil
	}

	for _, key := range keys {
		keyStr, ok := key.(string)
		if !ok {
			continue
		}
		fullPath := path
		if fullPath != "" {
			fullPath += "/"
		}
		fullPath += strings.TrimSuffix(keyStr, "/")

		if strings.HasSuffix(keyStr, "/") {
			// This is a directory, recurse
			nested, err := c.listRecursive(ctx, mount, fullPath)
			if err != nil {
				return nil, err
			}
			result[strings.TrimSuffix(keyStr, "/")] = nested
		} else {
			// This is a secret, read it
			data, err := c.readSecret(ctx, mount, fullPath)
			if err != nil {
				return nil, err
			}
			result[keyStr] = data
		}
	}

	return result, nil
}

func (c *Client) readSecret(ctx context.Context, mount, path string) (map[string]any, error) {
	secret, err := c.client.Logical().ReadWithContext(ctx, fmt.Sprintf("%s/data/%s", mount, path))
	if err != nil {
		return nil, fmt.Errorf("failed to read secret at %s: %w", path, err)
	}

	if secret == nil || secret.Data == nil {
		return nil, nil
	}

	data, ok := secret.Data["data"].(map[string]any)
	if !ok {
		return nil, nil
	}

	return data, nil
}

// splitMountPath splits a path into mount point and secret path
// e.g., "secret/data/myapp/config" -> "secret", "myapp/config"
func splitMountPath(path string) (string, string) {
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// ResolveMountPath detects the KV v2 mount for a path by querying /sys/mounts.
// Returns (mount, secretPath, error). For "satellite/slc/app/key" with mount "satellite/slc",
// returns ("satellite/slc", "app/key", nil).
func (c *Client) ResolveMountPath(ctx context.Context, path string) (string, string, error) {
	if err := c.ensureMountCache(ctx); err != nil {
		// Fall back to simple split if we can't query mounts
		mount, secretPath := splitMountPath(path)
		return mount, secretPath, nil
	}

	// Find longest matching mount (mounts are sorted by length descending)
	for _, mount := range c.mountCache {
		if strings.HasPrefix(path, mount+"/") {
			secretPath := strings.TrimPrefix(path, mount+"/")
			return mount, secretPath, nil
		}
		if path == mount {
			return mount, "", nil
		}
	}

	// No match found, fall back to simple split
	mount, secretPath := splitMountPath(path)
	return mount, secretPath, nil
}

// ensureMountCache fetches and caches KV v2 mounts if not already cached
func (c *Client) ensureMountCache(ctx context.Context) error {
	if c.mountCache != nil {
		return nil
	}

	mounts, err := c.client.Sys().ListMountsWithContext(ctx)
	if err != nil {
		return err
	}

	var kvMounts []string
	for path, mount := range mounts {
		// Check if it's a KV v2 mount
		if mount.Type == "kv" {
			if v, ok := mount.Options["version"]; ok && v == "2" {
				// Remove trailing slash from mount path
				kvMounts = append(kvMounts, strings.TrimSuffix(path, "/"))
			}
		}
	}

	// Sort by length descending so longer mounts match first
	sort.Slice(kvMounts, func(i, j int) bool {
		return len(kvMounts[i]) > len(kvMounts[j])
	})

	c.mountCache = kvMounts
	return nil
}

// ensureTrailingSlash adds a trailing slash to a path if it doesn't have one
func ensureTrailingSlash(path string) string {
	if path != "" && !strings.HasSuffix(path, "/") {
		return path + "/"
	}
	return path
}

// ReadSecretRaw reads a secret and returns the raw data (without transformation)
func (c *Client) ReadSecretRaw(ctx context.Context, path string) (map[string]any, error) {
	mount, secretPath, _ := c.ResolveMountPath(ctx, path)
	return c.readSecret(ctx, mount, secretPath)
}

// ReadSecretVersion reads a specific version of a secret
func (c *Client) ReadSecretVersion(ctx context.Context, path string, version int) (map[string]any, error) {
	mount, secretPath, _ := c.ResolveMountPath(ctx, path)
	return c.readSecretVersion(ctx, mount, secretPath, version)
}

func (c *Client) readSecretVersion(ctx context.Context, mount, path string, version int) (map[string]any, error) {
	// Use ReadWithDataWithContext to pass version as a query parameter
	versionParam := map[string][]string{
		"version": {fmt.Sprintf("%d", version)},
	}
	secret, err := c.client.Logical().ReadWithDataWithContext(ctx, fmt.Sprintf("%s/data/%s", mount, path), versionParam)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret version %d at %s: %w", version, path, err)
	}

	if secret == nil || secret.Data == nil {
		return nil, nil
	}

	data, ok := secret.Data["data"].(map[string]any)
	if !ok {
		return nil, nil
	}

	return data, nil
}

// WriteSecret writes data to a secret path
func (c *Client) WriteSecret(ctx context.Context, path string, data map[string]any) error {
	mount, secretPath, _ := c.ResolveMountPath(ctx, path)
	return c.WriteSecretWithMount(ctx, mount, secretPath, data)
}

// WriteSecretWithMount writes data to a secret path with an explicit mount point.
// Use this when the mount path contains slashes (e.g., "satellite/slc").
func (c *Client) WriteSecretWithMount(ctx context.Context, mount, path string, data map[string]any) error {
	_, err := c.client.Logical().WriteWithContext(ctx, fmt.Sprintf("%s/data/%s", mount, path), map[string]any{
		"data": data,
	})
	if err != nil {
		return fmt.Errorf("failed to write secret at %s/%s: %w", mount, path, err)
	}

	return nil
}

// WriteSecrets writes multiple secrets from a flattened map.
// Each key in the data map becomes a separate secret path under basePath,
// with the value stored as {"value": val}.
func (c *Client) WriteSecrets(ctx context.Context, basePath string, data map[string]any) (int, error) {
	mount, secretPath, _ := c.ResolveMountPath(ctx, basePath)
	return c.WriteSecretsWithMount(ctx, mount, secretPath, data)
}

// WriteSecretsWithMount writes multiple secrets with an explicit mount point.
// Use this when the mount path contains slashes (e.g., "satellite/slc").
func (c *Client) WriteSecretsWithMount(ctx context.Context, mount, basePath string, data map[string]any) (int, error) {
	// Sort keys for consistent ordering
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		secretPath := basePath + "/" + key
		// Convert value to string
		strValue := fmt.Sprintf("%v", data[key])
		secretData := map[string]any{
			"value": strValue,
		}
		if err := c.WriteSecretWithMount(ctx, mount, secretPath, secretData); err != nil {
			return 0, err
		}
	}

	return len(keys), nil
}

// DeleteSecret deletes a secret at the given path (all versions and metadata)
func (c *Client) DeleteSecret(ctx context.Context, path string) error {
	mount, secretPath, _ := c.ResolveMountPath(ctx, path)

	_, err := c.client.Logical().DeleteWithContext(ctx, fmt.Sprintf("%s/metadata/%s", mount, secretPath))
	if err != nil {
		return fmt.Errorf("failed to delete secret at %s: %w", path, err)
	}

	return nil
}

// SecretExists checks if a secret exists at the given path
func (c *Client) SecretExists(ctx context.Context, path string) (bool, error) {
	mount, secretPath, _ := c.ResolveMountPath(ctx, path)

	secret, err := c.client.Logical().ReadWithContext(ctx, fmt.Sprintf("%s/metadata/%s", mount, secretPath))
	if err != nil {
		return false, fmt.Errorf("failed to check secret at %s: %w", path, err)
	}

	return secret != nil && secret.Data != nil, nil
}

// ListSecretPaths recursively lists all secret paths under a given path
// Returns relative paths from the given base path
func (c *Client) ListSecretPaths(ctx context.Context, path string) ([]string, error) {
	mount, secretPath, _ := c.ResolveMountPath(ctx, path)
	return c.listSecretPathsRecursive(ctx, mount, secretPath, "")
}

func (c *Client) listSecretPathsRecursive(ctx context.Context, mount, basePath, relativePath string) ([]string, error) {
	var paths []string

	fullPath := basePath
	if relativePath != "" {
		fullPath = basePath + "/" + relativePath
	}

	secret, err := c.client.Logical().ListWithContext(ctx, fmt.Sprintf("%s/metadata/%s", mount, ensureTrailingSlash(fullPath)))
	if err != nil {
		return nil, fmt.Errorf("failed to list at %s: %w", fullPath, err)
	}

	if secret == nil || secret.Data == nil {
		return nil, nil
	}

	keys, ok := secret.Data["keys"].([]any)
	if !ok {
		return nil, nil
	}

	for _, key := range keys {
		keyStr, ok := key.(string)
		if !ok {
			continue
		}
		keyRelPath := keyStr
		if relativePath != "" {
			keyRelPath = relativePath + "/" + keyStr
		}

		if strings.HasSuffix(keyStr, "/") {
			// Directory - recurse
			subPaths, err := c.listSecretPathsRecursive(ctx, mount, basePath, strings.TrimSuffix(keyRelPath, "/"))
			if err != nil {
				return nil, err
			}
			paths = append(paths, subPaths...)
		} else {
			// Secret
			paths = append(paths, keyRelPath)
		}
	}

	return paths, nil
}

// IsDirectory checks if a path is a directory (has children) rather than a secret
func (c *Client) IsDirectory(ctx context.Context, path string) (bool, error) {
	mount, secretPath, _ := c.ResolveMountPath(ctx, path)

	secret, err := c.client.Logical().ListWithContext(ctx, fmt.Sprintf("%s/metadata/%s", mount, ensureTrailingSlash(secretPath)))
	if err != nil {
		return false, fmt.Errorf("failed to list at %s: %w", path, err)
	}

	return secret != nil && secret.Data != nil, nil
}

// ListDirectories lists immediate subdirectories at a path (non-recursive)
// Returns directory names (without trailing slash) and whether secrets exist at this level
func (c *Client) ListDirectories(ctx context.Context, path string) (dirs []string, hasSecrets bool, err error) {
	mount, secretPath, _ := c.ResolveMountPath(ctx, path)

	secret, err := c.client.Logical().ListWithContext(ctx, fmt.Sprintf("%s/metadata/%s", mount, ensureTrailingSlash(secretPath)))
	if err != nil {
		return nil, false, fmt.Errorf("failed to list at %s: %w", path, err)
	}

	if secret == nil || secret.Data == nil {
		return nil, false, nil
	}

	keys, ok := secret.Data["keys"].([]any)
	if !ok {
		return nil, false, nil
	}

	for _, key := range keys {
		keyStr, ok := key.(string)
		if !ok {
			continue
		}
		if strings.HasSuffix(keyStr, "/") {
			dirs = append(dirs, strings.TrimSuffix(keyStr, "/"))
		} else {
			hasSecrets = true
		}
	}

	return dirs, hasSecrets, nil
}

// SecretMetadata contains metadata about a secret
type SecretMetadata struct {
	CreatedTime    time.Time
	UpdatedTime    time.Time
	CurrentVersion int
	MaxVersions    int
	CustomMetadata map[string]string
}

// GetMetadata retrieves metadata for a secret
func (c *Client) GetMetadata(ctx context.Context, path string) (*SecretMetadata, error) {
	mount, secretPath, _ := c.ResolveMountPath(ctx, path)

	secret, err := c.client.Logical().ReadWithContext(ctx, fmt.Sprintf("%s/metadata/%s", mount, secretPath))
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata at %s: %w", path, err)
	}

	if secret == nil || secret.Data == nil {
		return nil, nil
	}

	metadata := &SecretMetadata{}

	if v, ok := secret.Data["current_version"].(json.Number); ok {
		if i, err := v.Int64(); err == nil {
			metadata.CurrentVersion = int(i)
		}
	}

	if v, ok := secret.Data["max_versions"].(json.Number); ok {
		if i, err := v.Int64(); err == nil {
			metadata.MaxVersions = int(i)
		}
	}

	if v, ok := secret.Data["created_time"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			metadata.CreatedTime = t
		}
	}

	if v, ok := secret.Data["updated_time"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			metadata.UpdatedTime = t
		}
	}

	if v, ok := secret.Data["custom_metadata"].(map[string]any); ok {
		metadata.CustomMetadata = make(map[string]string)
		for k, val := range v {
			if s, ok := val.(string); ok {
				metadata.CustomMetadata[k] = s
			}
		}
	}

	return metadata, nil
}

// VersionInfo contains info about a specific version of a secret
type VersionInfo struct {
	Version     int
	CreatedTime time.Time
	Destroyed   bool
	Deleted     bool
}

// GetVersionHistory retrieves the version history for a secret
// Returns a list of VersionInfo sorted by version descending (newest first)
func (c *Client) GetVersionHistory(ctx context.Context, path string) ([]VersionInfo, error) {
	mount, secretPath, _ := c.ResolveMountPath(ctx, path)

	secret, err := c.client.Logical().ReadWithContext(ctx, fmt.Sprintf("%s/metadata/%s", mount, secretPath))
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata at %s: %w", path, err)
	}

	if secret == nil || secret.Data == nil {
		return nil, nil
	}

	versions, ok := secret.Data["versions"].(map[string]any)
	if !ok {
		return nil, nil
	}

	var result []VersionInfo
	for versionStr, versionData := range versions {
		version, err := strconv.Atoi(versionStr)
		if err != nil {
			continue
		}

		info := VersionInfo{Version: version}

		if vd, ok := versionData.(map[string]any); ok {
			if ct, ok := vd["created_time"].(string); ok {
				if t, err := time.Parse(time.RFC3339Nano, ct); err == nil {
					info.CreatedTime = t
				}
			}
			if destroyed, ok := vd["destroyed"].(bool); ok {
				info.Destroyed = destroyed
			}
			if dt, ok := vd["deletion_time"].(string); ok && dt != "" {
				info.Deleted = true
			}
		}

		// Only include non-destroyed, non-deleted versions
		if !info.Destroyed && !info.Deleted {
			result = append(result, info)
		}
	}

	// Sort by version descending (newest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Version > result[j].Version
	})

	return result, nil
}
