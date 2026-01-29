package cmd

import (
	"context"
	"fmt"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var duplicatesCmd = &cobra.Command{
	Use:   "duplicates <path>",
	Short: "Find duplicate secret values",
	Long: `Find secrets with duplicate values under the given path.

Recursively checks all secrets and reports paths that share the same value.
Only the paths are printed, not the actual values.

Example:
  vlt duplicates secret/myapp
  # Finds all duplicate values under secret/myapp`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDuplicates(cmd.Context(), args[0])
	},
}

func init() {
	rootCmd.AddCommand(duplicatesCmd)
}

func runDuplicates(ctx context.Context, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	// Check if path exists first
	secrets, err := client.Get(ctx, path)
	if err != nil {
		return err
	}
	if len(secrets) == 0 {
		return fmt.Errorf("no secrets found at %s", path)
	}

	duplicates, err := client.FindDuplicates(ctx, path)
	if err != nil {
		return err
	}

	if len(duplicates) == 0 {
		fmt.Println("No duplicate values found.")
		return nil
	}

	for _, group := range duplicates {
		fmt.Println("Duplicate values found:")
		for _, p := range group.Paths {
			fmt.Printf("  %s\n", p)
		}
		fmt.Println()
	}

	return nil
}
