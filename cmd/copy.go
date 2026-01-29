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
	Short:   "Copy a secret or directory",
	Long: `Copy a secret or directory from one path to another.

Never overwrites existing secrets at the destination path.

Example:
  vlt copy secret/myapp/config secret/myapp/config-backup

  vlt copy secret/myapp secret/myapp-backup -r`,
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

	if err := client.Copy(ctx, src, dst); err != nil {
		return err
	}
	fmt.Printf("Copied %s -> %s\n", src, dst)
	return nil
}
