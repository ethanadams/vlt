# vlt

A CLI tool and Go library for managing secrets in HashiCorp Vault / OpenBao (KV v2).

## Installation

```bash
go install github.com/ethanadams/vlt@latest
```

Or build from source:

```bash
go build -o vlt .
```

## Configuration

Set the following environment variables:

```bash
export VAULT_ADDR="https://vault.example.com"
export VAULT_TOKEN="your-token"
# Or use a token file:
export VAULT_TOKEN_FILE="/path/to/token"
```

## Commands

### ls

List secrets and directories at a path.

```bash
# List contents
vlt ls secret/myapp

# List with metadata (version, updated time)
vlt ls secret/myapp -l
```

### get

Get secrets from a Vault path and print to stdout as YAML.

```bash
# Get all secrets under a path (recursive)
vlt get secret/myapp

# Get a specific secret
vlt get secret/myapp/config

# Get a specific key from a secret
vlt get secret/myapp/config apiKey
```

### add

Add a new secret at a path. Fails if the secret already exists (use `update` instead).

```bash
# Add with inline value
vlt add secret/myapp/apiKey "my-secret-value"

# Add from stdin
cat credentials.json | vlt add secret/myapp/gcp_sa -

# Add from stdin (implicit)
echo "secret" | vlt add secret/myapp/token
```

### update

Update an existing secret. Fails if the secret doesn't exist.

```bash
# Update with inline value
vlt update secret/myapp/apiKey "new-value"

# Update from stdin
cat new-credentials.json | vlt update secret/myapp/gcp_sa -
```

### rm

Remove secrets at a path.

```bash
# Delete a specific secret
vlt rm secret/myapp/config

# Delete all secrets under a path (requires -r)
vlt rm secret/myapp -r
```

### copy (cp)

Copy secrets.

```bash
# Copy a single secret
vlt copy secret/myapp/config secret/myapp/config-backup

# Copy all secrets under a path
vlt copy secret/myapp secret/myapp-backup -r
```

### mv

Move or rename secrets.

```bash
# Move a secret
vlt mv secret/old/path secret/new/path

# Move a directory of secrets
vlt mv secret/myapp secret/myapp-backup
```

### export

Export secrets to YAML files.

```bash
# Export to a single file
vlt export secret/myapp
# Creates myapp.yaml

# Export with custom output path
vlt export secret/myapp -o config.yaml

# Export recursively (creates directory structure)
vlt export secret/myapp -r
```

### import

Import secrets from a YAML file.

```bash
# Import secrets
vlt import secrets.yaml secret/myapp

# Preview without writing (dry-run)
vlt import secrets.yaml secret/myapp --dry-run

# Append filename to path (app-secrets.yaml -> secret/app/*)
vlt import app-secrets.yaml secret --append-name

# Import SOPS-encrypted file
vlt import --sops app-secrets.enc.yaml secret/myapp

# Nested mounts are auto-detected (e.g., satellite/slc)
vlt import --sops --append-name app-secrets.enc.yaml satellite/slc

# Update counterpart file with vault references
# Given app-secrets.yaml, updates app.yaml with refs like:
#   admin.password: ref+vault://secret/myapp/admin.password#value
vlt import app-secrets.yaml secret/myapp --update-counterpart
```

The `--sops` flag decrypts SOPS-encrypted files before importing. The `--update-counterpart` flag is useful for [vals](https://github.com/helmfile/vals) workflows where you maintain a config file with vault references instead of actual secrets.

### diff

Compare secrets between two Vault paths, or between a Vault path and a local YAML file.

```bash
# Compare staging and production
vlt diff secret/staging/app secret/prod/app

# Compare different versions of the same secret
vlt diff secret/myapp/config@1 secret/myapp/config@2

# Compare a previous version with current
vlt diff secret/myapp/config@1 secret/myapp/config

# Compare previous version with current (shorthand)
vlt diff secret/myapp/config@prev secret/myapp/config

# Compare previous versions of all secrets under a path
vlt diff secret/myapp@prev secret/myapp

# Compare directory state N changes ago (timeline-based)
# Shows cumulative changes across all secrets in the directory
vlt diff secret/myapp@-1 secret/myapp  # most recent change
vlt diff secret/myapp@-3 secret/myapp  # state 3 changes ago

# Output:
# Comparing secret/staging/app → secret/prod/app
#
# Only in secret/staging/app:
#   - debug.enabled
#
# Only in secret/prod/app:
#   + monitoring.apiKey
#
# Changed:
#   ~ database.password (32 → 48 chars)
#
# Unchanged: 15 keys

# Compare Vault with a local file (preview before import)
vlt diff secrets.yaml secret/myapp

# Compare SOPS-encrypted file with Vault
vlt diff --sops secrets.enc.yaml secret/myapp

# Compare two local files
vlt diff old-config.yaml new-config.yaml

# Show actual values (use with caution)
vlt diff config.yaml secret/myapp --show-values

# Show only counts
vlt diff secret/v1 secret/v2 --summary

# Exit code only (for scripting)
vlt diff secret/v1 secret/v2 --quiet && echo "identical"
```

Exit codes: 0 = identical, 1 = different, 2 = error.

### duplicates

Find duplicate secret values under a path.

```bash
# Find all duplicate values
vlt duplicates secret/myapp

# Output shows paths with matching values (not the values themselves)
# Duplicate values found:
#   secret/myapp/config.apiKey
#   secret/myapp/backup.apiKey
```

### edit

Edit secrets in your default editor (like `kubectl edit`).

```bash
# Edit a single secret
vlt edit secret/myapp/config

# Edit all secrets under a path
vlt edit secret/myapp

# Use a specific editor
EDITOR=nano vlt edit secret/myapp
```

Opens the secret(s) as YAML. If the path is a directory, all secrets under it are loaded for editing. After saving and closing, changes are written back to Vault (including deletions).

### history

Show version history for a secret or directory.

```bash
# Show version history for a single secret
vlt history secret/myapp/config
# v3  2024-01-30 10:15:23  (current)
# v2  2024-01-29 14:22:01
# v1  2024-01-28 09:00:00

# Show what changed each version
vlt history secret/myapp/config -v
# v3  2024-01-30 10:15:23  (current)
#     ~ password (32 → 48 chars)
# v2  2024-01-29 14:22:01
#     + api_key
# v1  2024-01-28 09:00:00
#     (initial version)

# Show actual values (use with caution)
vlt history secret/myapp/config --show-values
# v3  2024-01-30 10:15:23  (current)
#     ~ password: old-pass → new-pass
# v2  2024-01-29 14:22:01
#     + api_key: sk-abc123

# Show timeline of all changes in a directory
vlt history secret/myapp
# 2024-01-30 10:15:23  config      v2 → v3
# 2024-01-30 09:00:00  database    v1 → v2
# 2024-01-29 14:22:01  config      v1 → v2

# Limit entries shown
vlt history secret/myapp -n 5

# Show all versions (no limit)
vlt history secret/myapp --all
```

### tree

Display secrets in a tree view.

```bash
# Show tree structure
vlt tree secret/myapp
# myapp/
# ├── api/
# │   ├── key
# │   └── secret
# ├── database/
# │   ├── host
# │   └── password
# └── config
#
# 2 directories, 5 secrets

# Include version and timestamp metadata
vlt tree secret/myapp -l
# myapp/
# ├── api/
# │   ├── key  v3  2024-01-30 10:15:23
# │   └── secret  v1  2024-01-28 09:00:00
# ...
```

### snapshot

Create a point-in-time backup of all secrets under a path.

```bash
# Create a snapshot
vlt snapshot secret/myapp -o backup.yaml

# The snapshot includes version numbers and timestamps for each secret
# Example output file:
# path: secret/myapp
# created_at: 2024-01-30T10:15:23Z
# secrets:
#   config:
#     value: some-value
#     version: 3
#     updated: 2024-01-30T10:15:23Z
#   database/password:
#     value: db-secret
#     version: 1
#     updated: 2024-01-28T09:00:00Z
```

### restore

Restore secrets from a snapshot.

```bash
# Preview what would be restored (dry-run)
vlt restore backup.yaml secret/myapp --dry-run
# Preview of restore operation (dry-run):
# Added (1): + database/password
# Updated (1): ~ config
# Deleted (1): - extra-secret
# Summary: 1 added, 1 updated, 1 deleted, 2 unchanged

# Restore secrets
vlt restore backup.yaml secret/myapp

# Restore but don't delete secrets that aren't in the snapshot
vlt restore backup.yaml secret/myapp --no-delete

# Only restore if versions match (fail if secrets were modified since snapshot)
vlt restore backup.yaml secret/myapp --verify
```

By default, `restore` synchronizes the target path to match the snapshot exactly:
- Secrets in the snapshot but not in Vault are **added**
- Secrets that differ from the snapshot are **updated**
- Secrets in Vault but not in the snapshot are **deleted**

Use `--no-delete` to preserve extra secrets, and `--verify` to skip secrets whose versions have changed since the snapshot was taken.

## Library Usage

The `pkg/vault`, `pkg/config`, and `pkg/counterpart` packages can be imported by other Go modules:

```go
import (
    "context"
    "github.com/ethanadams/vlt/pkg/config"
    "github.com/ethanadams/vlt/pkg/counterpart"
    "github.com/ethanadams/vlt/pkg/vault"
)

func main() {
    ctx := context.Background()

    // Load config from environment
    cfg, _ := config.Load()

    // Or create directly
    cfg := &config.Config{
        VaultAddr:  "http://localhost:8200",
        VaultToken: "token",
    }

    client, _ := vault.NewClient(cfg)

    // Operations
    client.Add(ctx, "secret/app/key", "value")
    client.Update(ctx, "secret/app/key", "new-value")
    client.Copy(ctx, "secret/src", "secret/dst")
    client.Move(ctx, "secret/old", "secret/new")
    client.DeleteSecret(ctx, "secret/app/key")

    secrets, _ := client.Get(ctx, "secret/app")
    entries, _ := client.List(ctx, "secret/app")

    // Bulk operations
    client.CopyRecursive(ctx, "secret/src", "secret/dst")
    client.MoveRecursive(ctx, "secret/old", "secret/new")
    client.DeleteRecursive(ctx, "secret/app")

    // Import from map
    data := map[string]any{"admin": map[string]any{"password": "secret"}}
    client.Import(ctx, "secret/app", data)

    // Find duplicates
    dups, _ := client.FindDuplicates(ctx, "secret/app")

    // Update counterpart file with vault references
    keys := []string{"admin.password", "db.url"}
    counterpart.Update("app.yaml", "secret/myapp", keys)
}
```

## Development

### Build

```bash
make build          # Build binary
make install        # Install to GOPATH/bin
make clean          # Remove build artifacts
```

### Running Tests

**Unit tests** (no Docker required):
```bash
make test-unit          # Go unit tests for pure functions
```

**Go integration tests** (requires Docker, auto-manages Vault via testcontainers):
```bash
make test-go-integration  # Spins up Vault automatically per test
```

**CLI end-to-end tests** (requires running Vault server):
```bash
make docker-up      # Start test Vault server
make test-e2e       # CLI tests (flags, output formatting, workflows)
make docker-down    # Stop test server
```

**Recommended workflow**:
```bash
make test           # Unit + Go integration (requires Docker only, runs in CI)
make test-all       # Everything including CLI e2e tests
```

### Code Quality

```bash
make vet            # Run go vet
make lint           # Run golangci-lint (if installed)
make coverage       # Generate coverage report
```

### Continuous Integration

GitHub Actions automatically runs on every push/PR:
- **Lint**: `go vet` and `golangci-lint`
- **Unit Tests**: Go tests with race detection and coverage
- **Integration Tests**: Full test suite against Vault
- **Multi-platform Build**: Linux, macOS, Windows

### Project Structure

```
vlt/
├── main.go                     # Entry point
├── cmd/                        # CLI commands (Cobra)
│   ├── root.go                 # Root command, aliases
│   ├── ls.go, get.go           # Read operations
│   ├── add.go, update.go       # Write operations
│   ├── rm.go, mv.go, copy.go   # CRUD operations
│   ├── diff.go, history.go     # Comparison/history
│   ├── tree.go                 # Visual tree display
│   ├── export.go, import.go    # YAML import/export
│   ├── snapshot.go, restore.go # Backup/restore
│   ├── edit.go                 # Interactive editing
│   └── duplicates.go           # Find duplicates
├── pkg/
│   ├── config/config.go        # Configuration (env vars)
│   ├── counterpart/            # Counterpart file updates
│   │   └── counterpart.go      # Update YAML with vault refs
│   └── vault/
│       ├── client.go           # Vault API client (KV v2)
│       ├── operations.go       # High-level operations
│       ├── compare.go          # Diff/comparison utilities
│       ├── timeline.go         # Version history/timeline
│       ├── tree.go             # Tree structure building
│       ├── snapshot.go         # Snapshot/restore operations
│       └── flatten.go          # Nested map flattening
├── docker-compose.yml          # Test server (OpenBao)
└── test_e2e.sh                 # CLI end-to-end tests
```

## License

MIT
