# Feature Ideas

## Implemented ✓

### `vlt diff <path1> <path2>`
Compare secrets between paths/environments. Features:
- Compare two Vault paths (recursive)
- Compare Vault path with local YAML file
- Compare two local YAML files
- Compare different versions of a single secret using `@N` suffix (e.g., `secret/app/config@1` vs `secret/app/config@2`)
- `@prev` alias for previous version, works for both single secrets and directories
  - Single secret: `secret/app/config@prev` vs `secret/app/config`
  - Directory: `secret/app@prev` vs `secret/app` (compares previous version of each secret)
- `@-N` for directory timeline diffs: `secret/app@-3` shows state 3 changes ago across all secrets
  - Builds a timeline of all changes across all secrets in the directory
  - Useful for reviewing recent changes or identifying rollback points
- `--sops` flag for SOPS-encrypted files
- `--show-values` flag to display actual values
- `--summary` for counts only
- `--quiet` for scripting (exit codes: 0=identical, 1=different)

### `vlt import <file> <vault-path>`
Import secrets from YAML files to Vault. Features:
- SOPS decryption support (`--sops`)
- Dry-run mode (`--dry-run`)
- Auto-detect nested mounts
- Update counterpart files with vault refs (`--update-counterpart`)

### `vlt edit <path>`
Edit secrets in your default editor (like `kubectl edit`). Features:
- Opens secret(s) as YAML in $EDITOR (or $VISUAL, or vi)
- Single secret: edit just that secret's keys
- Directory: edit all secrets under the path recursively
- Detects changes and only updates if modified
- Shows added (`+`), changed (`~`), and deleted (`-`) keys
- Full sync: additions, changes, and deletions are all applied

### `vlt history <path>`
Show version history for a secret or directory. Features:
- Single secret: shows all versions with timestamps
- Directory: shows timeline of all changes across secrets
- `-v, --verbose`: show what changed each version (+added, ~changed, -deleted)
- `--show-values`: show actual secret values (use with caution)
- `-n, --limit N`: limit entries shown (default 10)
- `--all`: show all versions

```bash
vlt history secret/myapp/config
# v3  2024-01-30 10:15:23  (current)
# v2  2024-01-29 14:22:01
# v1  2024-01-28 09:00:00

vlt history secret/myapp/config -v
# v3  2024-01-30 10:15:23  (current)
#     ~ password (32 → 48 chars)
# v2  2024-01-29 14:22:01
#     + api_key
# v1  2024-01-28 09:00:00
#     (initial version)

vlt history secret/myapp/config --show-values
# v3  2024-01-30 10:15:23  (current)
#     ~ password: old-pass → new-pass
# v2  2024-01-29 14:22:01
#     + api_key: sk-abc123
# v1  2024-01-28 09:00:00
#     (initial version)

vlt history secret/myapp
# 2024-01-30 10:15:23  config      v2 → v3
# 2024-01-30 09:00:00  database    v1 → v2
# 2024-01-29 14:22:01  config      v1 → v2
```

### `vlt tree <path>`
Visual tree view of secret hierarchy. Features:
- Displays directories and secrets in a visual tree format
- `-l, --long`: show version and timestamp metadata
- Summary of directories and secrets count

```bash
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

vlt tree secret/myapp -l
# myapp/
# ├── api/
# │   ├── key  v3  2024-01-30 10:15:23
# │   └── secret  v1  2024-01-28 09:00:00
# ...
```

### `vlt snapshot <path> -o <file>`
Create point-in-time backup with version metadata.
```bash
vlt snapshot secret/myapp -o backup-2024-01-30.yaml
# Creates YAML file with:
#   path: secret/myapp
#   created_at: 2024-01-30T10:15:23Z
#   secrets:
#     config:
#       value: some-value
#       version: 3
#       updated: 2024-01-30T10:15:23Z
```

### `vlt restore <file> <path>`
Restore secrets from a snapshot with full synchronization.
```bash
vlt restore backup.yaml secret/myapp
# Restores all secrets from backup
# - Adds secrets that are in snapshot but not in Vault
# - Updates secrets that differ from snapshot
# - Deletes secrets that are in Vault but not in snapshot

vlt restore backup.yaml secret/myapp --verify
# Only restore if current versions match snapshot versions
# (skips secrets that were modified since snapshot)

vlt restore backup.yaml secret/myapp --no-delete
# Don't delete secrets that aren't in the snapshot

vlt restore backup.yaml secret/myapp --dry-run
# Preview what would be restored
```

---

## Version & History

### `vlt rollback <path> [version]`
Rollback a secret to a previous version.
```bash
vlt rollback secret/myapp/config        # rollback to previous version
vlt rollback secret/myapp/config 2      # rollback to specific version
vlt rollback secret/myapp -r            # rollback all secrets in directory to their previous versions
vlt rollback secret/myapp -r --dry-run  # preview what would be rolled back
```

### `vlt diff --since=<duration>`
Time-based diffs using duration syntax.
```bash
vlt diff secret/myapp --since=1h    # changes in last hour
vlt diff secret/myapp --since=7d    # changes in last week
vlt diff secret/myapp --since=30m   # changes in last 30 minutes
```

---

## Safety & Validation

### `vlt verify <file> <path>`
Verify local file matches what's in Vault. Useful for CI/CD pipelines.
```bash
vlt verify secrets.yaml secret/myapp
# ✓ Secrets match

vlt verify secrets.yaml secret/myapp --strict
# Also checks for extra keys in Vault not in file

# Exit codes: 0 = match, 1 = mismatch, 2 = error
```

### `vlt validate <file>`
Validate YAML structure before importing.
```bash
vlt validate secrets.yaml
# ✓ Valid YAML structure
# ✓ No empty values
# ✓ No duplicate keys
# ✓ All keys are valid secret paths

vlt validate secrets.yaml --sops  # validate SOPS-encrypted file
```

---

## Developer Experience

### `vlt env <path>`
Output secrets as shell export statements for sourcing.
```bash
eval $(vlt env secret/myapp)
# Exports: MYAPP_DATABASE_PASSWORD, MYAPP_API_KEY, etc.

vlt env secret/myapp --prefix=APP_  # custom prefix
vlt env secret/myapp --flat         # flatten nested: DATABASE_HOST instead of DATABASE.HOST
```

### `vlt dotenv <path>`
Output secrets in .env file format.
```bash
vlt dotenv secret/myapp              # output to stdout
vlt dotenv secret/myapp -o .env      # write to file
vlt dotenv secret/myapp --uppercase  # convert keys to UPPER_SNAKE_CASE
```

### `vlt bookmark <name> <path>`
Save frequently used paths for quick access.
```bash
vlt bookmark prod secret/prod/myapp
vlt bookmark staging secret/staging/myapp

vlt get @prod               # use bookmark
vlt diff @staging @prod     # compare bookmarks
vlt ls @prod                # list bookmark

vlt bookmark --list         # show all bookmarks
vlt bookmark --delete prod  # remove bookmark
```

---

## Bulk Operations

### `vlt rename <path> <old-key> <new-key>`
Rename a key within a secret (or recursively across secrets).
```bash
vlt rename secret/myapp/config apiKey api_key         # single secret
vlt rename secret/myapp apiKey api_key -r             # all secrets under path
vlt rename secret/myapp apiKey api_key -r --dry-run   # preview changes
```

### `vlt search <pattern>`
Search secret paths by regex pattern.
```bash
vlt search "prod.*database"
# secret/prod/app1/database
# secret/prod/app2/database

vlt search --keys "password"  # search within key names
```

### `vlt grep <pattern>`
Search within secret values (outputs paths only, never values).
```bash
vlt grep "postgres://" secret/
# secret/app1/database.url
# secret/app2/database.url

# Useful for finding secrets containing specific patterns
# (e.g., old hostnames, deprecated API endpoints)
```

---

## CI/CD & Environment Promotion

### `vlt promote <src> <dst>`
Promote secrets from one environment to another with safety checks.
```bash
vlt promote secret/staging/myapp secret/prod/myapp
# Shows diff, requires confirmation

vlt promote secret/staging/myapp secret/prod/myapp --dry-run
# Preview only

vlt promote secret/staging/myapp secret/prod/myapp \
  --exclude="debug.*" \
  --only="database.*,api.*"
# Filter which secrets to promote
```

### `vlt sync <src> <dst>`
Sync secrets between paths (like rsync for secrets).
```bash
vlt sync secret/template/app secret/prod/app
# Adds missing secrets, updates changed ones

vlt sync secret/template/app secret/prod/app --delete
# Also remove secrets in dst that aren't in src

vlt sync secret/template/app secret/prod/app --dry-run
# Preview changes
```

---

## Security & Auditing

### `vlt audit <path>`
Show access history for secrets (requires Vault audit logs).
```bash
vlt audit secret/myapp/config
# 2024-01-30 10:15:23  read   user:alice
# 2024-01-30 09:00:00  write  user:bob
# 2024-01-29 14:22:01  read   app:deployment

vlt audit secret/myapp -r --since=24h  # recursive, last 24 hours
```

### `vlt expired [path]`
Find secrets that haven't been rotated recently.
```bash
vlt expired secret/myapp --days=90
# Secrets not updated in 90+ days:
#   secret/myapp/legacy-api-key  (last updated: 180 days ago)
#   secret/myapp/old-password    (last updated: 95 days ago)
```

### `vlt weak [path]`
Detect potentially weak secrets.
```bash
vlt weak secret/myapp
# Potential issues found:
#   secret/myapp/password  - value is only 8 characters
#   secret/myapp/api-key   - value matches common pattern
#   secret/myapp/token     - value is empty
```

### `vlt refs <path>`
Find other secrets with the same value (by hash, never reveals values).
```bash
vlt refs secret/myapp/shared-key
# Same value found at:
#   secret/app1/api-key
#   secret/app2/api-key
#   secret/app3/api-key
# (Useful for finding shared secrets that need rotation)
```

---

## Advanced

### `vlt template <file>`
Render a template file with secrets injected.
```bash
vlt template config.yaml.tpl -o config.yaml
# Replaces {{ vault "secret/myapp/password" }} with actual values

vlt template config.yaml.tpl --env
# Output as environment variables instead of file
```

### `vlt watch <path>`
Watch for changes and run a command.
```bash
vlt watch secret/myapp -- kubectl rollout restart deployment/myapp
# Runs command whenever secrets under path change

vlt watch secret/myapp --interval=30s  # check every 30 seconds
```

### `vlt policy <path>`
Show or suggest Vault policies for a path.
```bash
vlt policy secret/myapp
# Suggests minimal policy for read/write access

vlt policy secret/myapp --readonly
# Suggests read-only policy
```
