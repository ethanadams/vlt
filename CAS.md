# Check-and-Set (CAS) for Atomic Operations

This document outlines how Vault KV v2's Check-and-Set mechanism could be used to make `vlt` operations atomic.

## Background

Vault KV v2 supports **optimistic locking** via Check-and-Set (CAS). This prevents unintentional overwrites when multiple clients modify the same secret concurrently.

**Current behavior**: `vlt` writes always succeed regardless of concurrent modifications. This can lead to lost updates in multi-user or automated environments.

## How CAS Works

When writing a secret, you can include a `cas` parameter specifying the expected current version:

```go
// Without CAS (current implementation)
payload := map[string]any{
    "data": secretData,
}

// With CAS
payload := map[string]any{
    "options": map[string]any{
        "cas": expectedVersion,
    },
    "data": secretData,
}
```

### CAS Values

| Value | Behavior |
|-------|----------|
| `0` | Write only succeeds if the key **does not exist** |
| `N` | Write only succeeds if current version is exactly `N` |
| omitted | Write always succeeds (current `vlt` behavior) |

### Error on Mismatch

When CAS fails, Vault returns an error:
```
check-and-set parameter did not match the current version
```

## What Vault Does NOT Support

- **Path-level locking**: No explicit `LOCK`/`UNLOCK` operations
- **Multi-key transactions**: Cannot atomically update multiple secrets together
- **Pessimistic locking**: No way to "hold" a lock while performing operations

## Proposed Implementation

### 1. New Client Method

```go
// WriteSecretWithCAS writes a secret with Check-and-Set protection.
// expectedVersion: 0 = must not exist, N = must be at version N
// Returns ErrVersionMismatch if CAS check fails.
func (c *Client) WriteSecretWithCAS(ctx context.Context, path string, data map[string]any, expectedVersion int) error {
    mount, secretPath, _ := c.ResolveMountPath(ctx, path)

    payload := map[string]any{
        "options": map[string]any{
            "cas": expectedVersion,
        },
        "data": data,
    }

    _, err := c.client.Logical().WriteWithContext(ctx,
        fmt.Sprintf("%s/data/%s", mount, secretPath), payload)

    if err != nil && strings.Contains(err.Error(), "check-and-set parameter did not match") {
        return ErrVersionMismatch
    }
    return err
}

var ErrVersionMismatch = errors.New("version mismatch: secret was modified by another process")
```

### 2. Command Integration

#### `add` Command
Use `cas: 0` for true atomicity (currently uses `SecretExists` check which has a race window):

```go
// Current (race condition possible):
exists, _ := client.SecretExists(ctx, path)
if exists {
    return errors.New("secret already exists")
}
client.WriteSecret(ctx, path, data)  // Could overwrite if created between check and write

// With CAS (atomic):
err := client.WriteSecretWithCAS(ctx, path, data, 0)
if errors.Is(err, ErrVersionMismatch) {
    return errors.New("secret already exists")
}
```

#### `update` Command
Optionally use CAS to prevent concurrent modification:

```go
// Get current version
metadata, _ := client.GetMetadata(ctx, path)

// Write with CAS
err := client.WriteSecretWithCAS(ctx, path, data, metadata.CurrentVersion)
if errors.Is(err, ErrVersionMismatch) {
    return errors.New("secret was modified by another process, please retry")
}
```

#### `restore` Command
Add `--cas` flag for atomic restore:

```bash
vlt restore backup.yaml secret/myapp --cas
```

Implementation:
```go
if opts.CAS {
    metadata, err := c.GetMetadata(ctx, fullPath)
    expectedVersion := 0
    if err == nil {
        expectedVersion = metadata.CurrentVersion
    }

    err = c.WriteSecretWithCAS(ctx, fullPath, data, expectedVersion)
    if errors.Is(err, ErrVersionMismatch) {
        return fmt.Errorf("secret %s was modified during restore", relPath)
    }
}
```

### 3. Library API

```go
// WriteOptions controls write behavior
type WriteOptions struct {
    CAS        *int  // nil = no CAS check, 0 = must not exist, N = must be version N
    CreateOnly bool  // Shorthand for CAS=0
}

// Usage
client.WriteSecretWithOptions(ctx, path, data, WriteOptions{
    CAS: &currentVersion,
})

client.WriteSecretWithOptions(ctx, path, data, WriteOptions{
    CreateOnly: true,  // Equivalent to CAS=0
})
```

## Use Cases

### 1. Prevent Accidental Overwrites
Multiple developers or automation systems modifying the same secrets:
```bash
# Developer A reads secret at v3
# Developer B reads secret at v3
# Developer A updates (now v4)
# Developer B updates with --cas -> FAILS (expected v3, found v4)
```

### 2. Atomic Create
Ensure a secret is only created once (e.g., initial setup):
```bash
vlt add secret/myapp/api-key "initial-key" --atomic
# Fails if secret already exists, even in race conditions
```

### 3. Safe Restore
Ensure no secrets were modified during a restore operation:
```bash
vlt restore backup.yaml secret/myapp --cas
# Fails immediately if any secret was modified mid-restore
```

### 4. CI/CD Pipelines
Prevent parallel deployments from clobbering each other:
```bash
# Pipeline A and B both try to update secrets
# Only one succeeds, other gets clear error
```

## Configuration Options

Vault supports requiring CAS at the mount or path level:

```bash
# Require CAS for entire mount
vault write secret/config cas_required=true

# Require CAS for specific path
vault kv metadata put -cas-required=true secret/critical-app
```

When `cas_required=true`, all writes without a CAS parameter will fail.

## Migration Path

1. **Phase 1**: Add `WriteSecretWithCAS` method (no CLI changes)
2. **Phase 2**: Add `--cas` flag to `restore` command
3. **Phase 3**: Add `--atomic` flag to `add` command (uses CAS=0)
4. **Phase 4**: Consider `--cas` for `update` and `import` commands
5. **Phase 5**: Consider global `--cas` flag or config option

## References

- [Vault KV v2 API Documentation](https://developer.hashicorp.com/vault/api-docs/secret/kv/kv-v2)
- [Versioned KV Secrets Tutorial](https://developer.hashicorp.com/vault/tutorials/secrets-management/versioned-kv)
- [KV Secrets Engine Documentation](https://developer.hashicorp.com/vault/docs/secrets/kv/kv-v2)
- [GitHub Issue: CAS Field Documentation](https://github.com/hashicorp/vault/issues/18724)
