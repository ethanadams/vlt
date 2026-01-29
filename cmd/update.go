package cmd

import (
	"context"
	"fmt"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update <path> [value]",
	Short: "Update a secret at a path",
	Long: `Update an existing secret at the given path.

The value can be provided as an argument or piped via stdin.

Example:
  vlt update secret/myapp/apiKey "new-secret-value"

  cat credentials.json | vlt update secret/myapp/gcp_sa -`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := readValueFromArgs(args, 1)
		if err != nil {
			return err
		}
		return runUpdate(cmd.Context(), args[0], value)
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(ctx context.Context, path, value string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	if err := client.Update(ctx, path, value); err != nil {
		return fmt.Errorf("%w (use 'add' to create new secrets)", err)
	}

	fmt.Printf("Updated secret at %s\n", path)
	return nil
}
