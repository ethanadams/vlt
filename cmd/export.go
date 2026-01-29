package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	exportOutput    string
	exportRecursive bool
)

var exportCmd = &cobra.Command{
	Use:   "export <path>",
	Short: "Export secrets from a Vault path to YAML",
	Long: `Export all secrets from a Vault path to a YAML file.

The output file is named after the last component of the path.

With --recursive, traverses all subdirectories and creates a local
directory structure mirroring Vault, with YAML files for each path.

Example:
  vlt export secret/myapp
  # Creates myapp.yaml

  vlt export secret/myapp --recursive
  # Creates myapp/ directory with nested structure`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runExport(cmd.Context(), args[0])
	},
}

func init() {
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "output file path (default: <name>.yaml)")
	exportCmd.Flags().BoolVarP(&exportRecursive, "recursive", "r", false, "recursively export all subdirectories")
	rootCmd.AddCommand(exportCmd)
}

func runExport(ctx context.Context, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	if exportRecursive {
		return runRecursiveExport(ctx, client, path, ".")
	}

	return exportPath(ctx, client, path, exportOutput)
}

func runRecursiveExport(ctx context.Context, client *vault.Client, vaultPath, localDir string) error {
	dirs, hasSecrets, err := client.ListDirectories(ctx, vaultPath)
	if err != nil {
		return err
	}

	// If this path has secrets, export them
	if hasSecrets {
		outputFile := filepath.Join(localDir, getParentKey(vaultPath)+".yaml")
		if err := exportPath(ctx, client, vaultPath, outputFile); err != nil {
			return err
		}
	}

	// Recurse into subdirectories
	for _, dir := range dirs {
		// Validate directory name to prevent path traversal
		if strings.Contains(dir, "..") || strings.HasPrefix(dir, "/") || strings.Contains(dir, string(filepath.Separator)) {
			return fmt.Errorf("invalid directory name from Vault: %q", dir)
		}

		subVaultPath := vaultPath + "/" + dir
		subLocalDir := filepath.Join(localDir, dir)

		// Create local directory
		if err := os.MkdirAll(subLocalDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", subLocalDir, err)
		}

		if err := runRecursiveExport(ctx, client, subVaultPath, subLocalDir); err != nil {
			return err
		}
	}

	return nil
}

func exportPath(ctx context.Context, client *vault.Client, path, outputFile string) error {
	secrets, err := client.Export(ctx, path)
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

	if outputFile == "" {
		outputFile = getParentKey(path) + ".yaml"
	}

	if err := os.WriteFile(outputFile, yamlData, 0600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Exported secrets to %s\n", outputFile)
	return nil
}

// getParentKey extracts the last component from a path
// e.g., "secret/myapp/config" -> "config"
func getParentKey(path string) string {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
