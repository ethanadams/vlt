package cmd

import (
	"context"
	"fmt"
	"sort"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var (
	restoreDryRun    bool
	restoreVerify    bool
	restoreNoDelete  bool
)

var restoreCmd = &cobra.Command{
	Use:   "restore <file> <path>",
	Short: "Restore secrets from a snapshot",
	Long: `Restore secrets from a previously created snapshot.

By default, secrets that exist in Vault but not in the snapshot will be deleted.
Use --no-delete to preserve extra secrets.

Use --verify to only restore if secret versions match the snapshot
(fails if secrets were modified since the snapshot was taken).

Examples:
  vlt restore backup.yaml secret/myapp
  vlt restore backup.yaml secret/myapp --dry-run    # preview changes
  vlt restore backup.yaml secret/myapp --verify     # fail if modified
  vlt restore backup.yaml secret/myapp --no-delete  # don't delete extra secrets`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRestore(cmd.Context(), args[0], args[1])
	},
}

func init() {
	restoreCmd.Flags().BoolVar(&restoreDryRun, "dry-run", false, "preview changes without applying")
	restoreCmd.Flags().BoolVar(&restoreVerify, "verify", false, "only restore if versions match snapshot")
	restoreCmd.Flags().BoolVar(&restoreNoDelete, "no-delete", false, "don't delete secrets not in snapshot")
	rootCmd.AddCommand(restoreCmd)
}

func runRestore(ctx context.Context, snapshotFile, targetPath string) error {
	// Load snapshot
	snapshot, err := LoadSnapshot(snapshotFile)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	opts := vault.RestoreOptions{
		DryRun:      restoreDryRun,
		Verify:      restoreVerify,
		DeleteExtra: !restoreNoDelete,
	}

	result, err := client.RestoreSnapshot(ctx, snapshot, targetPath, opts)
	if err != nil {
		return err
	}

	// Print results
	printRestoreResult(result, restoreDryRun)

	return nil
}

func printRestoreResult(result *vault.RestoreResult, dryRun bool) {
	action := ""
	if dryRun {
		action = " (dry-run)"
		fmt.Printf("Preview of restore operation%s:\n\n", action)
	} else {
		fmt.Printf("Restore completed:\n\n")
	}

	if len(result.Added) > 0 {
		fmt.Printf("Added (%d):\n", len(result.Added))
		sort.Strings(result.Added)
		for _, p := range result.Added {
			fmt.Printf("  + %s\n", p)
		}
		fmt.Println()
	}

	if len(result.Updated) > 0 {
		fmt.Printf("Updated (%d):\n", len(result.Updated))
		sort.Strings(result.Updated)
		for _, p := range result.Updated {
			fmt.Printf("  ~ %s\n", p)
		}
		fmt.Println()
	}

	if len(result.Deleted) > 0 {
		fmt.Printf("Deleted (%d):\n", len(result.Deleted))
		sort.Strings(result.Deleted)
		for _, p := range result.Deleted {
			fmt.Printf("  - %s\n", p)
		}
		fmt.Println()
	}

	if len(result.Skipped) > 0 {
		fmt.Printf("Skipped (version mismatch) (%d):\n", len(result.Skipped))
		sort.Strings(result.Skipped)
		for _, p := range result.Skipped {
			fmt.Printf("  ! %s\n", p)
		}
		fmt.Println()
	}

	// Summary
	fmt.Printf("Summary: %d added, %d updated, %d deleted, %d unchanged",
		len(result.Added), len(result.Updated), len(result.Deleted), len(result.Unchanged))
	if len(result.Skipped) > 0 {
		fmt.Printf(", %d skipped", len(result.Skipped))
	}
	fmt.Println()

	if dryRun && result.HasChanges() {
		fmt.Println("\nRun without --dry-run to apply these changes.")
	}
}
