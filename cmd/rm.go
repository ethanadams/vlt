package cmd

import (
	"context"
	"fmt"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var rmRecursive bool

var rmCmd = &cobra.Command{
	Use:   "rm <path>",
	Short: "Remove secrets at a path",
	Long: `Remove secrets at the given path.

If the path is a secret, deletes that secret.
If the path is a directory, requires -r flag to delete recursively.

Example:
  vlt rm secret/myapp/config
  # Deletes the secret at secret/myapp/config

  vlt rm secret/myapp -r
  # Deletes all secrets under secret/myapp`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRm(cmd.Context(), args[0])
	},
}

func init() {
	rmCmd.Flags().BoolVarP(&rmRecursive, "recursive", "r", false, "recursively delete all secrets under the path")
	rootCmd.AddCommand(rmCmd)
}

func runRm(ctx context.Context, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	// Check if this path is a secret
	exists, err := client.SecretExists(ctx, path)
	if err != nil {
		return err
	}

	if exists {
		if err := client.DeleteSecret(ctx, path); err != nil {
			return err
		}
		fmt.Printf("Deleted %s\n", path)
		return nil
	}

	// Check if it's a directory
	dirs, hasSecrets, err := client.ListDirectories(ctx, path)
	if err != nil {
		return err
	}

	if !hasSecrets && len(dirs) == 0 {
		return fmt.Errorf("no secrets found at %s", path)
	}

	// It's a directory, require -r flag
	if !rmRecursive {
		return fmt.Errorf("cannot remove %s: is a directory (use -r to remove recursively)", path)
	}

	result, err := client.DeleteRecursive(ctx, path)
	if err != nil {
		return err
	}

	for _, deleted := range result.Deleted {
		fmt.Printf("Deleted %s\n", deleted)
	}
	return nil
}
