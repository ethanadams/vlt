package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var (
	rollbackRecursive bool
	rollbackDryRun    bool
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback <path> [version]",
	Short: "Roll back a secret to a previous version",
	Long: `Roll back a secret to a previous version.

Without a version argument, rolls back to the immediately previous version.
With a version number, rolls back to that specific version.
Use -r to rollback all secrets in a directory to their previous versions.

The rollback creates a new version with the old data (non-destructive).

Examples:
  vlt rollback secret/myapp/config
  # Roll back to previous version

  vlt rollback secret/myapp/config 2
  # Roll back to version 2

  vlt rollback secret/myapp -r
  # Roll back all secrets under myapp to their previous versions

  vlt rollback secret/myapp/config --dry-run
  # Preview what would change`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		version := 0
		if len(args) > 1 {
			v, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid version number: %s", args[1])
			}
			version = v
		}
		return runRollback(cmd.Context(), args[0], version)
	},
}

func init() {
	rollbackCmd.Flags().BoolVarP(&rollbackRecursive, "recursive", "r", false, "rollback all secrets under the path")
	rollbackCmd.Flags().BoolVar(&rollbackDryRun, "dry-run", false, "preview changes without applying")
	rootCmd.AddCommand(rollbackCmd)
}

func runRollback(ctx context.Context, path string, version int) error {
	initColor()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	if rollbackRecursive {
		return runRollbackRecursive(ctx, client, path)
	}

	return runRollbackSingle(ctx, client, path, version)
}

func runRollbackSingle(ctx context.Context, client *vault.Client, path string, version int) error {
	// Resolve the path
	resolved, err := client.ResolvePath(ctx, path)
	if err != nil {
		return fmt.Errorf("secret not found: %s", path)
	}
	if resolved.Key != "" {
		return fmt.Errorf("cannot rollback a single key, specify the secret path: %s", resolved.SecretPath)
	}

	if rollbackDryRun {
		return previewRollback(ctx, client, resolved.SecretPath, version)
	}

	result, err := client.Rollback(ctx, resolved.SecretPath, version)
	if err != nil {
		return err
	}

	fmt.Printf("Rolled back %s: v%d → v%d\n", result.Path, result.OldVersion, result.NewVersion)
	if len(result.Changes) > 0 {
		fmt.Println()
		for _, change := range result.Changes {
			fmt.Printf("  %s\n", formatRollbackChange(change))
		}
	}

	return nil
}

func runRollbackRecursive(ctx context.Context, client *vault.Client, path string) error {
	if rollbackDryRun {
		return previewRollbackRecursive(ctx, client, path)
	}

	result, err := client.RollbackRecursive(ctx, path)
	if err != nil {
		return err
	}

	for _, r := range result.Results {
		fmt.Printf("Rolled back %s: v%d → v%d\n", r.Path, r.OldVersion, r.NewVersion)
	}

	if len(result.Skipped) > 0 {
		fmt.Printf("\nSkipped %d secrets (at version 1, no previous version)\n", len(result.Skipped))
	}

	fmt.Printf("\nRolled back %d secrets\n", len(result.Results))

	return nil
}

func previewRollback(ctx context.Context, client *vault.Client, path string, version int) error {
	// Get current metadata
	metadata, err := client.GetMetadata(ctx, path)
	if err != nil {
		return err
	}
	if metadata == nil {
		return fmt.Errorf("secret not found at %s", path)
	}

	targetVersion := version
	if targetVersion == 0 {
		if metadata.CurrentVersion <= 1 {
			return fmt.Errorf("no previous version to rollback to (current version is %d)", metadata.CurrentVersion)
		}
		targetVersion = metadata.CurrentVersion - 1
	}

	changes, err := client.CompareVersions(ctx, path, metadata.CurrentVersion, targetVersion)
	if err != nil {
		return err
	}

	fmt.Printf("Preview: rollback %s v%d → v%d (dry-run)\n", path, metadata.CurrentVersion, targetVersion)
	if len(changes) > 0 {
		fmt.Println()
		for _, change := range changes {
			fmt.Printf("  %s\n", formatRollbackChange(change))
		}
	} else {
		fmt.Println("\n  No changes (versions are identical)")
	}

	fmt.Println("\nRun without --dry-run to apply.")

	return nil
}

func previewRollbackRecursive(ctx context.Context, client *vault.Client, path string) error {
	secretPaths, err := client.ListSecretPaths(ctx, path)
	if err != nil {
		return err
	}

	if len(secretPaths) == 0 {
		return fmt.Errorf("no secrets found under %s", path)
	}

	fmt.Printf("Preview: rollback %s (dry-run)\n\n", path)

	rollbackCount := 0
	skipCount := 0

	for _, relPath := range secretPaths {
		fullPath := path + "/" + relPath
		metadata, err := client.GetMetadata(ctx, fullPath)
		if err != nil || metadata == nil || metadata.CurrentVersion <= 1 {
			skipCount++
			continue
		}

		targetVersion := metadata.CurrentVersion - 1
		changes, err := client.CompareVersions(ctx, fullPath, metadata.CurrentVersion, targetVersion)
		if err != nil {
			skipCount++
			continue
		}

		fmt.Printf("%s: v%d → v%d\n", relPath, metadata.CurrentVersion, targetVersion)
		for _, change := range changes {
			fmt.Printf("  %s\n", formatRollbackChange(change))
		}
		rollbackCount++
	}

	if skipCount > 0 {
		fmt.Printf("\nWould skip %d secrets (at version 1)\n", skipCount)
	}
	fmt.Printf("Would rollback %d secrets\n", rollbackCount)
	fmt.Println("\nRun without --dry-run to apply.")

	return nil
}

func formatRollbackChange(change vault.VersionChange) string {
	switch change.Type {
	case vault.ChangeAdded:
		return colorGreen(fmt.Sprintf("+ %s", change.Key))
	case vault.ChangeModified:
		return colorYellow(fmt.Sprintf("~ %s (%d → %d chars)", change.Key, change.OldLength, change.NewLength))
	case vault.ChangeDeleted:
		return colorRed(fmt.Sprintf("- %s", change.Key))
	default:
		return change.Key
	}
}
