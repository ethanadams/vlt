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
	Use:     "get <path>",
	Aliases: []string{"read"},
	Short:   "Get secrets from a Vault path and print to stdout",
	Long: `Get secrets from a Vault path and print to stdout.

The path can refer to:
  - A specific key: secret/myapp/db/password → prints just the password value
  - A secret: secret/myapp/db → prints all keys in the db secret as YAML
  - A directory: secret/myapp → prints all secrets under myapp as YAML

Path resolution:
  The command walks up the path to find the secret, so secret/myapp/db/password
  will find the secret at secret/myapp/db and return the "password" key.

Example:
  vlt get secret/myapp
  # Prints all secrets under myapp as YAML

  vlt get secret/myapp/db
  # Prints all keys in the db secret as YAML

  vlt get secret/myapp/db/password
  # Prints just the value of password`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGet(cmd.Context(), args[0])
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}

func runGet(ctx context.Context, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	// Try to resolve the path to determine if it's a key, secret, or directory
	resolved, err := client.ResolvePath(ctx, path)
	if err == nil {
		if resolved.Key != "" {
			// Path resolves to a specific key - print just that value
			value, err := client.ReadKey(ctx, resolved.SecretPath, resolved.Key)
			if err != nil {
				return err
			}
			if isStructuredOutput() {
				return printOutput(map[string]any{resolved.Key: value})
			}
			fmt.Println(value)
			return nil
		}
		// Path is a secret - print all its keys
		data, err := client.ReadSecretRaw(ctx, resolved.SecretPath)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return fmt.Errorf("no keys found in secret at %s", path)
		}
		if isStructuredOutput() {
			return printOutput(data)
		}
		yamlData, err := yaml.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML: %w", err)
		}
		fmt.Print(string(yamlData))
		return nil
	}

	// Path doesn't resolve to a secret - try as a directory
	secrets, err := client.Get(ctx, path)
	if err != nil {
		return err
	}

	if len(secrets) == 0 {
		return fmt.Errorf("no secrets found at %s", path)
	}

	if isStructuredOutput() {
		return printOutput(secrets)
	}

	yamlData, err := yaml.Marshal(secrets)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	fmt.Print(string(yamlData))
	return nil
}
