package cmd

import (
	"context"
	"fmt"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <path> [value]",
	Short: "Add a new secret at a path",
	Long: `Add a new secret at the given path.

The value can be provided as an argument or piped via stdin.
Fails if the secret already exists (use 'update' to modify existing secrets).

Example:
  vlt add secret/myapp/apiKey "my-secret-value"

  cat credentials.json | vlt add secret/myapp/gcp_sa -

  echo "secret" | vlt add secret/myapp/token`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := readValueFromArgs(args, 1)
		if err != nil {
			return err
		}
		return runAdd(cmd.Context(), args[0], value)
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}

func runAdd(ctx context.Context, path, value string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	if err := client.Add(ctx, path, value); err != nil {
		return err
	}

	fmt.Printf("Added secret at %s\n", path)
	return nil
}
