# Add Command Skill

Add a new CLI command to vlt following established patterns.

## Usage

```
/add-command <name> [description]
```

Examples:
- `/add-command rollback` - Add rollback command
- `/add-command env "Output secrets as shell export statements"`

## Instructions

When adding a new command, follow these steps:

### 1. Create the command file

Create `cmd/<name>.go` with this structure:

```go
package cmd

import (
	"context"
	"fmt"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var <name>Cmd = &cobra.Command{
	Use:   "<name> <path>",
	Short: "<Short description>",
	Long: `<Long description with examples>

Examples:
  vlt <name> secret/myapp
  vlt <name> secret/myapp --flag`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return run<Name>(cmd.Context(), args[0])
	},
}

func init() {
	// Add flags here if needed:
	// <name>Cmd.Flags().BoolVarP(&<name>Recursive, "recursive", "r", false, "description")
	rootCmd.AddCommand(<name>Cmd)
}

func run<Name>(ctx context.Context, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	// Implementation here
	_ = client // use client
	fmt.Printf("Running %s on %s\n", "<name>", path)

	return nil
}
```

### 2. Add vault operation (if needed)

If the command needs a reusable operation, add it to `pkg/vault/operations.go`:

```go
// <Name> does X for secrets at the given path.
func (c *Client) <Name>(ctx context.Context, path string) error {
	// Implementation
	return nil
}
```

### 3. Update documentation

Add the command to the table in `CLAUDE.md`:

```markdown
| `<name>` | `<flags>` | <Description> |
```

### 4. Add e2e test

Add test cases to `test_e2e.sh` in the appropriate section:

```bash
# <name> command
./vlt add secret/e2e/<name>-test "value" 2>/dev/null
if ./vlt <name> secret/e2e/<name>-test 2>/dev/null; then
    pass "<name>: basic usage"
else
    fail "<name>: basic usage"
fi
```

### 5. Check IDEAS.md

If this command is listed in IDEAS.md, move it to the "Implemented ✓" section with updated documentation.

## Checklist

Before marking complete, verify:
- [ ] Command file created in `cmd/`
- [ ] Command added to `rootCmd` in `init()`
- [ ] Flags follow conventions (`-r` for recursive, `-l` for long, `-o` for output)
- [ ] Error messages are clear and actionable
- [ ] CLAUDE.md updated with command
- [ ] Test added to `test_e2e.sh`
- [ ] `go build` succeeds
- [ ] `go vet ./...` passes

## Flag Conventions

Follow these naming conventions:
- `-r, --recursive` - For directory operations
- `-l, --long` - Detailed/verbose output
- `-o, --output` - Output file path
- `-n, --limit` - Limit number of results
- `-v, --verbose` - Show more detail
- `--dry-run` - Preview without making changes
- `--quiet, -q` - Minimal output, for scripting

## Common Patterns

**Reading secrets:**
```go
secrets, err := client.Get(ctx, path)
if err != nil {
    return err
}
```

**Checking if path exists:**
```go
exists, err := client.SecretExists(ctx, path)
if err != nil {
    return err
}
if !exists {
    return fmt.Errorf("secret not found at %s", path)
}
```

**Handling directories vs secrets:**
```go
isDir, err := client.IsDirectory(ctx, path)
if err != nil {
    return err
}
if isDir {
    // Handle directory
} else {
    // Handle single secret
}
```

**Output formatting:**
```go
// YAML output
data, _ := yaml.Marshal(secrets)
fmt.Println(string(data))

// Table output
fmt.Printf("%-30s %s\n", "PATH", "VALUE")
```
