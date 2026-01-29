package cmd

import (
	"context"
	"fmt"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var (
	historyVerbose    bool
	historyLimit      int
	historyAll        bool
	historyShowValues bool
)

var historyCmd = &cobra.Command{
	Use:   "history <path>",
	Short: "Show version history for secrets",
	Long: `Show version history for a secret or directory.

For a single secret, shows all versions with timestamps.
For a directory, shows a timeline of all changes across secrets.

Examples:
  vlt history secret/myapp/config
  vlt history secret/myapp/config -v            # show what changed
  vlt history secret/myapp/config --show-values # show actual values
  vlt history secret/myapp                      # directory timeline
  vlt history secret/myapp -n 5                 # last 5 entries
  vlt history secret/myapp --all                # no limit`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHistory(cmd.Context(), args[0])
	},
}

func init() {
	historyCmd.Flags().BoolVarP(&historyVerbose, "verbose", "v", false, "show what changed each version")
	historyCmd.Flags().IntVarP(&historyLimit, "limit", "n", 10, "limit number of entries shown")
	historyCmd.Flags().BoolVar(&historyAll, "all", false, "show all versions (no limit)")
	historyCmd.Flags().BoolVar(&historyShowValues, "show-values", false, "show actual secret values (use with caution)")
	rootCmd.AddCommand(historyCmd)
}

func runHistory(ctx context.Context, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	// Check if path is a directory or single secret
	isDir, err := client.IsDirectory(ctx, path)
	if err != nil {
		// If error, might be a single secret - try that first
		isDir = false
	}

	if isDir {
		return showDirectoryHistory(ctx, client, path)
	}
	return showSecretHistory(ctx, client, path)
}

func showSecretHistory(ctx context.Context, client *vault.Client, path string) error {
	versions, err := client.GetVersionHistory(ctx, path)
	if err != nil {
		return err
	}

	if len(versions) == 0 {
		return fmt.Errorf("no versions found at %s", path)
	}

	// Apply limit
	limit := historyLimit
	if historyAll {
		limit = len(versions)
	}
	if limit > len(versions) {
		limit = len(versions)
	}

	fmt.Printf("History for %s:\n\n", path)

	for i, v := range versions[:limit] {
		current := ""
		if i == 0 {
			current = "  (current)"
		}

		fmt.Printf("v%-3d  %s%s\n", v.Version, v.CreatedTime.Local().Format("2006-01-02 15:04:05"), current)

		// Verbose mode: show what changed
		if (historyVerbose || historyShowValues) && i < limit-1 {
			// Compare this version with previous
			nextVersion := versions[i+1]
			changes, err := client.CompareVersions(ctx, path, nextVersion.Version, v.Version)
			if err == nil && len(changes) > 0 {
				for _, change := range changes {
					fmt.Printf("      %s\n", formatVersionChange(change, historyShowValues))
				}
			}
		} else if (historyVerbose || historyShowValues) && v.Version == 1 {
			fmt.Printf("      (initial version)\n")
		}
	}

	if limit < len(versions) {
		fmt.Printf("\n... and %d more versions (use --all to see all)\n", len(versions)-limit)
	}

	return nil
}

func showDirectoryHistory(ctx context.Context, client *vault.Client, path string) error {
	// Use vault.GetTimeline to get the timeline
	timeline, err := client.GetTimeline(ctx, path)
	if err != nil {
		return err
	}

	// Apply limit
	limit := historyLimit
	if historyAll {
		limit = len(timeline)
	}
	if limit > len(timeline) {
		limit = len(timeline)
	}

	fmt.Printf("History for %s:\n\n", path)

	for _, entry := range timeline[:limit] {
		action := fmt.Sprintf("v%d → v%d", entry.Version-1, entry.Version)
		if entry.IsCreation {
			action = "v1 (created)"
		}

		fmt.Printf("%s  %-20s  %s\n",
			entry.Time.Local().Format("2006-01-02 15:04:05"),
			entry.SecretPath,
			action,
		)

		// Verbose mode for directories
		if (historyVerbose || historyShowValues) && !entry.IsCreation {
			changes, err := client.CompareVersions(ctx, entry.FullPath, entry.Version-1, entry.Version)
			if err == nil && len(changes) > 0 {
				for _, change := range changes {
					fmt.Printf("                             %s\n", formatVersionChange(change, historyShowValues))
				}
			}
		}
	}

	if limit < len(timeline) {
		fmt.Printf("\n... and %d more entries (use --all to see all)\n", len(timeline)-limit)
	}

	return nil
}

// formatVersionChange formats a VersionChange for display
func formatVersionChange(change vault.VersionChange, showValues bool) string {
	switch change.Type {
	case vault.ChangeAdded:
		if showValues {
			return fmt.Sprintf("+ %s: %s", change.Key, change.NewValue)
		}
		return fmt.Sprintf("+ %s", change.Key)
	case vault.ChangeModified:
		if showValues {
			return fmt.Sprintf("~ %s: %s → %s", change.Key, change.OldValue, change.NewValue)
		}
		return fmt.Sprintf("~ %s (%d → %d chars)", change.Key, change.OldLength, change.NewLength)
	case vault.ChangeDeleted:
		if showValues {
			return fmt.Sprintf("- %s: %s", change.Key, change.OldValue)
		}
		return fmt.Sprintf("- %s", change.Key)
	default:
		return change.Key
	}
}
