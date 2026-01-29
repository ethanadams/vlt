# vlt - Vault CLI Tool

## Overview

`vlt` is a CLI tool and reusable Go library for managing secrets in HashiCorp Vault / OpenBao (KV v2). Built with Go and Cobra.

## Project Structure

```
vlt/
├── main.go                 # Entry point, calls cmd.Execute()
├── cmd/                    # Cobra commands
│   ├── root.go            # Root command setup
│   ├── helpers.go         # Shared utilities (readValueFromArgs)
│   ├── ls.go              # List secrets/directories
│   ├── get.go             # Get secrets (stdout, YAML)
│   ├── add.go             # Add new secret
│   ├── update.go          # Update existing secret
│   ├── rm.go              # Remove secrets
│   ├── mv.go              # Move/rename secrets
│   ├── copy.go            # Copy secrets
│   ├── export.go          # Export to YAML files
│   ├── import.go          # Import from YAML files
│   ├── duplicates.go      # Find duplicate values
│   ├── diff.go            # Compare secrets between paths
│   ├── edit.go            # Edit secret in $EDITOR
│   ├── history.go         # Show version history
│   ├── tree.go            # Visual tree display
│   ├── snapshot.go        # Create point-in-time backup
│   └── restore.go         # Restore from snapshot
├── pkg/                    # Public library packages (importable)
│   ├── config/config.go   # Loads VAULT_ADDR, VAULT_TOKEN from env
│   ├── counterpart/       # Update companion YAML files with vault refs
│   │   └── counterpart.go
│   └── vault/
│       ├── client.go      # Vault API wrapper (KV v2)
│       ├── operations.go  # High-level operations (Add, Get, Copy, Move, etc.)
│       ├── compare.go     # Diff/comparison utilities (CompareSecrets, CompareVersions)
│       ├── timeline.go    # Version history/timeline (GetTimeline, GetPrevVersions)
│       ├── tree.go        # Tree structure (GetTree, TreeNode)
│       ├── snapshot.go    # Snapshot/restore (CreateSnapshot, RestoreSnapshot)
│       └── flatten.go     # Flatten nested maps to dot-notation
└── go.mod                 # Module: github.com/ethanadams/vlt
```

## Commands

| Command | Flags | Description |
|---------|-------|-------------|
| `ls <path>` | `-l` | List secrets/dirs, `-l` shows metadata (path required) |
| `get` | | Get secrets as YAML (recursive by default) |
| `diff` | `--summary`, `--keys-only`, `-q`, `--sops`, `--show-values` | Compare secrets/versions between paths or with local file (see Version Syntax below) |
| `add` | | Add secret (value from arg or stdin) |
| `update` | | Update existing secret |
| `rm` | `-r` | Remove secret, `-r` for directories |
| `mv` | | Move/rename (handles dirs automatically) |
| `copy` | `-r` | Copy secret, `-r` for directories |
| `export` | `-r`, `-o` | Export to YAML files |
| `import` | `--dry-run`, `--append-name`, `--name`, `--sops`, `--update-counterpart` | Import from YAML files (SOPS support, auto-detects mounts) |
| `duplicates` | | Find duplicate values (recursive) |
| `edit` | | Edit a secret in $EDITOR (like kubectl edit) |
| `history` | `-v`, `-n`, `--all`, `--show-values` | Show version history for secrets |
| `tree` | `-l` | Display secrets in a tree view |
| `snapshot` | `-o` | Create point-in-time backup of secrets (required: -o) |
| `restore` | `--dry-run`, `--verify`, `--no-delete` | Restore secrets from a snapshot |

### Diff Version Syntax

The `diff` command supports version specifiers for comparing historical states:

| Syntax | Single Secret | Directory | Description |
|--------|--------------|-----------|-------------|
| `@N` | ✓ | ✗ | Specific version (e.g., `@3` for version 3) |
| `@prev` | ✓ | ✓ | Previous version (per-secret for directories) |
| `@-N` | ✗ | ✓ | State N changes ago, timeline-based (e.g., `@-2` for 2 changes ago) |

Examples:
```bash
# Single secret: compare version 1 with current
vlt diff secret/app/config@1 secret/app/config

# Single secret: previous version shorthand
vlt diff secret/app/config@prev secret/app/config

# Directory: previous version of each secret
vlt diff secret/app@prev secret/app

# Directory: state 3 changes ago (cumulative timeline)
vlt diff secret/app@-3 secret/app
```

The `@-N` syntax builds a timeline of all changes across all secrets in the directory and shows what the cumulative state looked like N changes ago. This is useful for auditing recent changes or reviewing a rollback point.

## Code Patterns

### Command Structure
```go
var cmdCmd = &cobra.Command{
    Use:   "cmd <args>",
    Short: "Short description",
    Long:  `Long description with examples`,
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        return runCmd(cmd.Context(), args[0])
    },
}

func init() {
    cmdCmd.Flags().BoolVarP(&flagVar, "flag", "f", false, "description")
    rootCmd.AddCommand(cmdCmd)
}

func runCmd(ctx context.Context, arg string) error {
    cfg, err := config.Load()
    if err != nil {
        return err
    }
    client, err := vault.NewClient(cfg)
    if err != nil {
        return err
    }
    // ... implementation
}
```

### High-Level Operations (pkg/vault/operations.go)
- `Add(ctx, path, value)` - Add a new secret (fails if already exists)
- `Update(ctx, path, value)` - Update existing secret (fails if not found)
- `Get(ctx, path)` - Get all secrets recursively as nested map
- `GetValue(ctx, path, key)` - Get specific key from a secret
- `List(ctx, path)` - List entries (dirs and secrets) at path
- `ListWithMetadata(ctx, path)` - List with version/timestamp metadata
- `Copy(ctx, src, dst)` - Copy single secret
- `CopyRecursive(ctx, src, dst)` - Copy all secrets under path
- `Move(ctx, src, dst)` - Move single secret
- `MoveRecursive(ctx, src, dst)` - Move all secrets under path
- `DeleteRecursive(ctx, path)` - Delete all secrets under path
- `Export(ctx, path)` - Export secrets as nested map
- `Import(ctx, basePath, data)` - Import from nested map (auto-detects mount)
- `FindDuplicates(ctx, path)` - Find secrets with duplicate values

### Low-Level Client Methods (pkg/vault/client.go)
- `ListSecrets(ctx, path)` - Recursive list, returns nested map
- `ListSecretPaths(ctx, path)` - Returns []string of relative paths
- `ListDirectories(ctx, path)` - Returns dirs, hasSecrets, err
- `ReadSecretRaw(ctx, path)` - Read single secret data
- `ReadSecretVersion(ctx, path, version)` - Read specific version of a secret
- `WriteSecret(ctx, path, data)` - Write secret (auto-detects mount)
- `WriteSecrets(ctx, basePath, data)` - Bulk write from flattened map
- `DeleteSecret(ctx, path)` - Delete secret (and metadata)
- `SecretExists(ctx, path)` - Check if secret exists
- `IsDirectory(ctx, path)` - Check if path is directory
- `GetMetadata(ctx, path)` - Get secret metadata (version, times)
- `GetVersionHistory(ctx, path)` - Get version history for a secret (version, created time, destroyed/deleted flags)
- `ResolveMountPath(ctx, path)` - Auto-detect KV v2 mount from path

Note: Mounts are auto-detected by querying /sys/mounts. Nested mounts like "satellite/slc" are handled automatically.

### Comparison Functions (pkg/vault/compare.go)
- `vault.CompareSecrets(secrets1, secrets2)` - Compare two flattened secret maps, returns DiffResult
- `client.CompareVersions(ctx, path, oldVer, newVer)` - Compare two versions of a secret
- `vault.ParseVersionedPath(path)` - Parse path@version syntax, returns (basePath, VersionSpec)
- `vault.FlattenAndExtractValues(data, forDirectory)` - Flatten and extract .value fields

### Timeline Functions (pkg/vault/timeline.go)
- `client.GetTimeline(ctx, path)` - Get chronological timeline of all changes under a path
- `client.GetPrevVersions(ctx, path)` - Get previous version of each secret under a path
- `client.GetStateAtChangesAgo(ctx, path, changesAgo)` - Get directory state N changes ago
- `client.GetSecretAtVersion(ctx, path, version, isPrev)` - Get secret at specific version

### Tree Functions (pkg/vault/tree.go)
- `client.GetTree(ctx, path)` - Build tree structure of secrets under a path
- `client.GetTreeWithMetadata(ctx, path)` - Build tree with version/timestamp metadata
- `TreeNode.Walk(callback)` - Traverse tree, callback receives (node, depth, isLast)
- `TreeNode.CountSecrets()` - Count total secrets in tree
- `TreeNode.CountDirs()` - Count total directories in tree

### Snapshot Functions (pkg/vault/snapshot.go)
- `client.CreateSnapshot(ctx, path)` - Create point-in-time backup of all secrets under a path
- `client.RestoreSnapshot(ctx, snapshot, targetPath, opts)` - Restore secrets from a snapshot
- `RestoreResult.HasChanges()` - Check if restore would make any changes
- `RestoreResult.TotalChanges()` - Get total number of changes

### Utility Functions
- `vault.Flatten(data)` - Convert nested map to flat dot-notation keys

### Counterpart Package (pkg/counterpart)
- `DeriveFilename(sourcePath)` - Derive counterpart filename (e.g., "app-secrets.yaml" -> "app.yaml")
- `CleanFilename(path)` - Extract clean name from filename
- `Update(path, vaultPath, keys)` - Update YAML file with vault refs (ref+vault://...)
- `FormatRef(vaultPath, key)` - Format a single vault reference string

### Secrets Structure
Secrets are stored with `{"value": "..."}` format. The `expandSecrets` function in client.go transforms this for display.

## Safety Behaviors

Commands are designed to fail safely:
- `add` fails if secret already exists (use `update` instead)
- `update` fails if secret does not exist (use `add` for new secrets)
- `copy`/`mv` fail if destination already exists
- `get`, `export`, `diff`, `duplicates` fail on non-existent paths
- `rm` without `-r` fails on directories

## Flag Conventions

- `-r, --recursive` - For directory operations (rm, copy, export)
- `-l, --long` - Detailed output (ls)
- `-o, --output` - Output file path (export)
- Stdout commands (get, duplicates) are recursive by default
- File-writing commands require `-r` for recursive

## Build & Test

```bash
go build -o vlt .
./vlt --help

# Run tests
make test              # Unit + Go integration tests (requires Docker only)
make test-e2e          # CLI end-to-end tests (requires: docker compose up -d)
make test-all          # Everything
```

## Environment Variables

- `VAULT_ADDR` - Vault server address (required)
- `VAULT_TOKEN` - Vault token (required, or use VAULT_TOKEN_FILE)
- `VAULT_TOKEN_FILE` - Path to file containing token

## Using as a Library

The `pkg/` packages are public and can be imported by other Go modules:

```go
import (
    "github.com/ethanadams/vlt/pkg/config"
    "github.com/ethanadams/vlt/pkg/counterpart"
    "github.com/ethanadams/vlt/pkg/vault"
)

// Load config from environment
cfg, _ := config.Load()

// Or create config directly
cfg := &config.Config{VaultAddr: "https://vault:8200", VaultToken: "token"}

// Create client
client, _ := vault.NewClient(cfg)

// High-level operations
client.Add(ctx, "secret/myapp/apikey", "secret-value")
client.Update(ctx, "secret/myapp/apikey", "new-value")
secrets, _ := client.Get(ctx, "secret/myapp")
client.Copy(ctx, "secret/myapp/config", "secret/myapp/config-backup")
client.Move(ctx, "secret/old", "secret/new")
client.DeleteRecursive(ctx, "secret/myapp/old")

// Import from nested map (e.g., parsed YAML)
data := map[string]any{
    "admin": map[string]any{
        "password": "secret",
    },
}
client.Import(ctx, "secret/myapp", data)

// Find duplicate values
duplicates, _ := client.FindDuplicates(ctx, "secret/myapp")

// Update counterpart file with vault references
// Useful for vals/helmfile workflows
keys := []string{"admin.password", "db.url"}
counterpart.Update("app.yaml", "secret/myapp", keys)
// app.yaml now contains: admin.password: ref+vault://secret/myapp/admin.password#value
```
