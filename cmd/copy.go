package cmd

import (
	"context"
	"fmt"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var copyRecursive bool

var copyCmd = &cobra.Command{
	Use:     "copy <source> <destination>",
	Aliases: []string{"cp"},
	Short:   "Copy a key, secret, or directory",
	Long: `Copy a key, secret, or directory from one path to another.

Supports copying:
  - A single key to another secret (or same secret with new name)
  - An entire secret to a new path
  - A directory of secrets to a new path (with -r flag)

Never overwrites existing keys or secrets at the destination.

Example:
  vlt copy secret/myapp/db/password secret/myapp/config/db_password
  # Copy a key to another secret

  vlt copy secret/myapp/db secret/myapp/db-backup
  # Copy an entire secret

  vlt copy secret/myapp secret/myapp-backup -r
  # Copy all secrets under a directory`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCopy(cmd.Context(), args[0], args[1])
	},
}

func init() {
	copyCmd.Flags().BoolVarP(&copyRecursive, "recursive", "r", false, "recursively copy all secrets under the path")
	rootCmd.AddCommand(copyCmd)
}

func runCopy(ctx context.Context, src, dst string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	if copyRecursive {
		count, err := client.CopyRecursive(ctx, src, dst)
		if err != nil {
			return err
		}
		fmt.Printf("Copied %d secrets\n", count)
		return nil
	}

	// Try to resolve the source
	resolved, err := client.ResolvePath(ctx, src)
	if err != nil {
		return fmt.Errorf("source not found: %s", src)
	}

	if err := client.Copy(ctx, src, dst); err != nil {
		return err
	}

	if resolved.Key != "" {
		fmt.Printf("Copied key %s -> %s\n", src, dst)
	} else {
		fmt.Printf("Copied secret %s -> %s\n", src, dst)
	}
	return nil
}
