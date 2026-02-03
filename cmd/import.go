package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/counterpart"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/getsops/sops/v3/decrypt"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	importDryRun            bool
	importAppendName        bool
	importName              string
	importUpdateCounterpart bool
	importMount             string
	importSops              bool
)

var importCmd = &cobra.Command{
	Use:   "import <yaml-file> <vault-path>",
	Short: "Import secrets from a YAML file to Vault",
	Long: `Import secrets from a YAML file to Vault KV v2.

The YAML is flattened to dot-notation keys and stored in a single secret.

Example YAML structure:
  db:
    password: secret123
    host: localhost
  api:
    key: abc123

Results in a single secret at vault-path with keys:
  db.password: secret123
  db.host: localhost
  api.key: abc123

Example:
  vlt import secrets.yaml secret/myapp
  # Creates secret/myapp with all keys from secrets.yaml

  vlt import secrets.yaml secret/myapp --dry-run
  # Preview what would be written without making changes

  vlt import config-secrets.yaml secret --append-name
  # Derives name from filename: secret/config

  vlt import app-secrets.yaml secret/myapp --update-counterpart
  # After import, updates app.yaml with vault refs

  vlt import --sops app-secrets.enc.yaml secret/myapp
  # Decrypt SOPS-encrypted file before importing`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runImport(cmd.Context(), args[0], args[1])
	},
}

func init() {
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "preview secrets without writing to Vault")
	importCmd.Flags().BoolVar(&importAppendName, "append-name", false, "append cleaned filename to vault path")
	importCmd.Flags().StringVar(&importName, "name", "", "override the derived name (use with --append-name)")
	importCmd.Flags().BoolVar(&importUpdateCounterpart, "update-counterpart", false, "update counterpart YAML file with vault references")
	importCmd.Flags().StringVar(&importMount, "mount", "", "KV v2 mount path (default: first path segment)")
	importCmd.Flags().BoolVar(&importSops, "sops", false, "decrypt SOPS-encrypted file before importing")
	rootCmd.AddCommand(importCmd)
}

func runImport(ctx context.Context, yamlFile, vaultPath string) error {
	// Read and parse YAML file
	var content []byte
	var err error

	if importSops {
		// Decrypt SOPS-encrypted file
		content, err = decrypt.File(yamlFile, "yaml")
		if err != nil {
			return fmt.Errorf("failed to decrypt SOPS file: %w", err)
		}
	} else {
		content, err = os.ReadFile(yamlFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
	}

	var data map[string]any
	if err := yaml.Unmarshal(content, &data); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Append cleaned filename to vault path if requested
	if importAppendName {
		name := importName
		if name == "" {
			name = counterpart.CleanFilename(yamlFile)
		}
		vaultPath = vaultPath + "/" + name
	}

	// Build full path (mount override + vault path)
	fullPath := vaultPath
	if importMount != "" {
		fullPath = importMount + "/" + vaultPath
	}

	// Flatten for counterpart update (uses dot-notation paths)
	flattened := vault.Flatten(data)
	keys := make([]string, 0, len(flattened))
	for k := range flattened {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if importDryRun {
		printImportDryRun(fullPath, data)
		if importUpdateCounterpart {
			printCounterpartDryRun(yamlFile, fullPath, keys)
		}
		return nil
	}

	// Load config and create client
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	// Import secrets (mount is auto-detected from path)
	count, err := client.Import(ctx, fullPath, data)
	if err != nil {
		return err
	}

	fmt.Printf("Successfully wrote %d keys to %s\n", count, fullPath)

	// Update counterpart file if requested
	if importUpdateCounterpart {
		counterpartPath := counterpart.DeriveFilename(yamlFile)
		result, err := counterpart.Update(counterpartPath, fullPath, keys)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update counterpart file: %v\n", err)
		} else if result.Updated {
			absPath, _ := filepath.Abs(counterpartPath)
			fmt.Printf("Updated %s with %d vault references\n", absPath, result.Keys)
		} else {
			absPath, _ := filepath.Abs(counterpartPath)
			fmt.Printf("Counterpart file %s does not exist, skipping\n", absPath)
		}
	}

	return nil
}

func printImportDryRun(path string, data map[string]any) {
	// Flatten the data to show what will actually be written
	flattened := vault.Flatten(data)

	// Sort keys for consistent output
	keys := make([]string, 0, len(flattened))
	for k := range flattened {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Printf("[dry-run] Would write to Vault secret: %s\n", path)
	fmt.Printf("[dry-run] %d keys:\n", len(keys))

	for _, k := range keys {
		v := flattened[k]
		switch val := v.(type) {
		case string:
			fmt.Printf("  %s = <string, %d chars>\n", k, len(val))
		default:
			fmt.Printf("  %s = <%T>\n", k, v)
		}
	}
}

func printCounterpartDryRun(yamlFile, vaultPath string, keys []string) {
	counterpartPath := counterpart.DeriveFilename(yamlFile)
	absPath, _ := filepath.Abs(counterpartPath)

	if _, err := os.Stat(counterpartPath); os.IsNotExist(err) {
		fmt.Printf("[dry-run] Counterpart file %s does not exist, would skip\n", absPath)
		return
	}

	fmt.Printf("[dry-run] Would update %s with vault references:\n", absPath)
	for _, k := range keys {
		fmt.Printf("  %s: %s\n", k, counterpart.FormatRef(vaultPath, k))
	}
}

