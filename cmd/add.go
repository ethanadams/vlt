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
	Short: "Add a new key to a secret",
	Long: `Add a new key-value pair to a secret.

The path format is: secret/path/key
  - The parent path (secret/path) is the secret location
  - The last segment (key) is the key name within that secret

The value can be provided as an argument or piped via stdin.
Fails if the key already exists (use 'update' to modify existing keys).

Example:
  vlt add secret/myapp/db/password "my-secret-value"
  # Adds key "password" to secret at secret/myapp/db

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

	secretPath, key := vault.ParseWritePath(path)
	fmt.Printf("Added key %q to secret at %s\n", key, secretPath)
	return nil
}
