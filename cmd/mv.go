package cmd

import (
	"context"
	"fmt"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var mvCmd = &cobra.Command{
	Use:   "mv <source> <destination>",
	Short: "Move or rename a key, secret, or directory",
	Long: `Move or rename a key, secret, or directory from one path to another.

Supports moving:
  - A key from one secret to another (or rename within same secret)
  - An entire secret to a new path
  - A directory of secrets to a new path

Never overwrites existing keys or secrets at the destination.

Examples:
  vlt mv secret/myapp/db/password secret/myapp/db/pass
  # Rename a key within a secret

  vlt mv secret/myapp/db/password secret/myapp/config/db_password
  # Move a key to a different secret

  vlt mv secret/myapp/db secret/myapp/database
  # Move an entire secret

  vlt mv secret/myapp secret/myapp-backup
  # Move all secrets under a directory`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMv(cmd.Context(), args[0], args[1])
	},
}

func init() {
	rootCmd.AddCommand(mvCmd)
}

func runMv(ctx context.Context, src, dst string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	// Try to resolve the source path
	resolved, err := client.ResolvePath(ctx, src)
	if err == nil {
		// Source exists - check if it's a key or secret
		if resolved.Key != "" {
			// Moving a key
			if err := client.Move(ctx, src, dst); err != nil {
				return err
			}
			fmt.Printf("Moved key %s -> %s\n", src, dst)
			return nil
		}

		// Moving a whole secret
		if err := client.Move(ctx, src, dst); err != nil {
			return err
		}
		fmt.Printf("Moved secret %s -> %s\n", src, dst)
		return nil
	}

	// Source didn't resolve to a secret - check if it's a directory
	isDir, err := client.IsDirectory(ctx, src)
	if err != nil {
		return fmt.Errorf("source not found: %s", src)
	}

	if isDir {
		count, err := client.MoveRecursive(ctx, src, dst)
		if err != nil {
			return err
		}
		fmt.Printf("Moved %d secrets from %s -> %s\n", count, src, dst)
		return nil
	}

	return fmt.Errorf("source not found: %s", src)
}
