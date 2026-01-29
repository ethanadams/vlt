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
	Short: "Move or rename a secret or directory",
	Long: `Move or rename a secret or directory from one path to another.

Never overwrites existing secrets at the destination path.
When moving a directory, all secrets within it are moved.

Examples:
  vlt mv secret/abc/123 secret/def/xyz/123
  vlt mv secret/old-name secret/new-name
  vlt mv secret/myapp/config secret/myapp/config-backup`,
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

	// Check if source is a directory
	isDir, err := client.IsDirectory(ctx, src)
	if err != nil {
		return err
	}

	if isDir {
		count, err := client.MoveRecursive(ctx, src, dst)
		if err != nil {
			return err
		}
		fmt.Printf("Moved %d secrets from %s -> %s\n", count, src, dst)
		return nil
	}

	if err := client.Move(ctx, src, dst); err != nil {
		return err
	}
	fmt.Printf("Moved %s -> %s\n", src, dst)
	return nil
}
