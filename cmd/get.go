package cmd

import (
	"context"
	"fmt"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var getCmd = &cobra.Command{
	Use:   "get <path> [key]",
	Short: "Get secrets from a Vault path and print to stdout",
	Long: `Get secrets from a Vault path and print to stdout.

Recursively traverses all subdirectories by default.
Outputs YAML. Optionally specify a key to get just that value.

Example:
  vlt get secret/myapp
  # Prints all secrets under myapp as YAML

  vlt get secret/myapp/config
  # Prints all keys in the config secret as YAML

  vlt get secret/myapp/config apiKey
  # Prints just the value of apiKey`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := ""
		if len(args) == 2 {
			key = args[1]
		}
		return runGet(cmd.Context(), args[0], key)
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}

func runGet(ctx context.Context, path, key string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	if key != "" {
		return getKeyValue(ctx, client, path, key)
	}

	return getPath(ctx, client, path)
}

func getPath(ctx context.Context, client *vault.Client, path string) error {
	secrets, err := client.Get(ctx, path)
	if err != nil {
		return err
	}

	if len(secrets) == 0 {
		return fmt.Errorf("no secrets found at %s", path)
	}

	yamlData, err := yaml.Marshal(secrets)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	fmt.Print(string(yamlData))
	return nil
}

func getKeyValue(ctx context.Context, client *vault.Client, path, key string) error {
	value, err := client.GetValue(ctx, path, key)
	if err != nil {
		return err
	}

	switch v := value.(type) {
	case string:
		fmt.Println(v)
	case map[string]any, []any:
		yamlData, err := yaml.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML: %w", err)
		}
		fmt.Print(string(yamlData))
	default:
		fmt.Println(v)
	}

	return nil
}
