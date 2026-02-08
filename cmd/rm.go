package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var rmRecursive bool

var rmCmd = &cobra.Command{
	Use:     "rm <path>",
	Aliases: []string{"delete"},
	Short:   "Remove a key, secret, or directory",
	Long: `Remove a key, secret, or directory at the given path.

The path is resolved to determine what to delete:
  - If path resolves to a key: deletes that key from the secret
  - If path is a secret: deletes the entire secret
  - If path is a directory: requires -r flag to delete recursively

Example:
  vlt rm secret/myapp/db/password
  # Deletes the "password" key from the db secret

  vlt rm secret/myapp/db
  # Deletes the entire db secret

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

	// When -r is specified, check for directory first
	if rmRecursive {
		dirs, hasSecrets, err := client.ListDirectories(ctx, path)
		if err == nil && (hasSecrets || len(dirs) > 0) {
			result, err := client.DeleteRecursive(ctx, path)
			if err != nil {
				return err
			}
			if result.Count > 1 {
				fmt.Fprintf(os.Stderr, "\r\033[K")
			}
			fmt.Printf("Deleted %d secrets from %s\n", result.Count, path)
			return nil
		}
	}

	// Try to resolve the path to see if it's a key within a secret
	resolved, err := client.ResolvePath(ctx, path)
	if err == nil {
		if resolved.Key != "" {
			// Path resolves to a key - delete just that key
			if err := client.DeleteKey(ctx, resolved.SecretPath, resolved.Key); err != nil {
				return err
			}
			fmt.Printf("Deleted key %q from %s\n", resolved.Key, resolved.SecretPath)
			return nil
		}
		// Path is a secret - delete it
		if err := client.DeleteSecret(ctx, resolved.SecretPath); err != nil {
			return err
		}
		fmt.Printf("Deleted secret at %s\n", resolved.SecretPath)
		return nil
	}

	// Path didn't resolve to a secret - check if it's a directory
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

	if result.Count > 1 {
		fmt.Fprintf(os.Stderr, "\r\033[K")
	}
	fmt.Printf("Deleted %d secrets from %s\n", result.Count, path)
	return nil
}
