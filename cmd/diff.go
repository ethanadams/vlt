package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/getsops/sops/v3/decrypt"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	diffSummary    bool
	diffKeysOnly   bool
	diffQuiet      bool
	diffSops       bool
	diffShowValues bool
)

var diffCmd = &cobra.Command{
	Use:   "diff <path1> <path2>",
	Short: "Compare secrets between two paths",
	Long: `Compare secrets between two Vault paths or a Vault path and a local file.

Shows keys that exist only in one path, keys with different values,
and a count of unchanged keys. Use --show-values to display actual values.

If a path exists as a local file, it will be read as YAML. Use --sops
to decrypt SOPS-encrypted files.

Version comparison:
  @N    - Compare specific version (single secrets only)
  @prev - Compare previous version (works for both single secrets and directories)
  @-N   - Compare state N changes ago (directories only, based on change timeline)

For directories:
  @prev compares the previous version of each secret
  @-N builds a timeline of all changes and shows the state N changes ago

Exit codes:
  0 - paths are identical
  1 - paths differ
  2 - error occurred

Example:
  vlt diff secret/staging/app secret/prod/app
  # Compare staging and prod environments

  vlt diff secret/myapp/config@1 secret/myapp/config@2
  # Compare version 1 with version 2 of a single secret

  vlt diff secret/myapp/config@prev secret/myapp/config
  # Compare previous version with current

  vlt diff secret/myapp@prev secret/myapp
  # Compare previous version of each secret under myapp

  vlt diff secret/myapp@-1 secret/myapp
  # See what changed in the most recent change across all secrets

  vlt diff secret/myapp@-3 secret/myapp
  # See cumulative changes from the last 3 changes

  vlt diff secret/myapp config.yaml
  # Compare Vault secrets with a local YAML file

  vlt diff --sops secrets.enc.yaml secret/myapp
  # Compare SOPS-encrypted file with Vault

  vlt diff secret/myapp secret/myapp-backup --summary
  # Show only counts

  vlt diff secret/v1 secret/v2 --quiet
  # Exit code only, for scripting`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDiff(cmd.Context(), args[0], args[1])
	},
}

func init() {
	diffCmd.Flags().BoolVar(&diffSummary, "summary", false, "only show counts, no details")
	diffCmd.Flags().BoolVar(&diffKeysOnly, "keys-only", false, "don't show value length changes")
	diffCmd.Flags().BoolVarP(&diffQuiet, "quiet", "q", false, "exit code only, no output")
	diffCmd.Flags().BoolVar(&diffSops, "sops", false, "decrypt SOPS-encrypted files")
	diffCmd.Flags().BoolVar(&diffShowValues, "show-values", false, "show actual secret values (use with caution)")
	rootCmd.AddCommand(diffCmd)
}

// Use vault.DiffResult, vault.DiffEntry, vault.ChangedEntry from pkg/vault/compare.go

func runDiff(ctx context.Context, path1, path2 string) error {
	// Check if either path is a local file
	path1IsFile := isLocalFile(path1)
	path2IsFile := isLocalFile(path2)

	// Only need Vault client if at least one path is a Vault path
	var client *vault.Client
	if !path1IsFile || !path2IsFile {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		client, err = vault.NewClient(cfg)
		if err != nil {
			return err
		}
	}

	result, err := comparePaths(ctx, client, path1, path2, path1IsFile, path2IsFile)
	if err != nil {
		return err
	}

	if !diffQuiet {
		printDiffResult(path1, path2, result)
	}

	if result.HasDifferences() {
		os.Exit(1)
	}
	return nil
}

// isLocalFile checks if the path exists as a local file
func isLocalFile(path string) bool {
	// Quick heuristic: if it contains common YAML extensions, check if file exists
	// Also check for paths that look like files (contain . in the last segment)
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".json") {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func comparePaths(ctx context.Context, client *vault.Client, path1, path2 string, path1IsFile, path2IsFile bool) (*vault.DiffResult, error) {
	// Get secrets from both paths
	secrets1, err := getSecretsFromSource(ctx, client, path1, path1IsFile)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path1, err)
	}

	secrets2, err := getSecretsFromSource(ctx, client, path2, path2IsFile)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path2, err)
	}

	return vault.CompareSecrets(secrets1, secrets2), nil
}

// getSecretsFromSource retrieves secrets from either a Vault path or a local file
func getSecretsFromSource(ctx context.Context, client *vault.Client, path string, isFile bool) (map[string]any, error) {
	if isFile {
		return getSecretsFromFile(path)
	}
	return getSecretsFromVault(ctx, client, path)
}

// getSecretsFromVault retrieves all secrets under a Vault path as a flat key->value map
// Supports @version suffix for reading specific versions (e.g., secret/myapp/config@1)
// Supports @prev alias for the previous version (works for both single secrets and directories)
// Supports @-N for N changes ago (directories only, based on change timeline)
// Note: Numeric version (@N) is only supported for single secrets, not directories
func getSecretsFromVault(ctx context.Context, client *vault.Client, path string) (map[string]any, error) {
	// Parse @version suffix using vault package
	basePath, spec := vault.ParseVersionedPath(path)

	// If no version specified, use the simple recursive Get
	if !spec.HasVersion() {
		secrets, err := client.Get(ctx, basePath)
		if err != nil {
			return nil, err
		}
		if len(secrets) == 0 {
			return nil, fmt.Errorf("no secrets found at %s", basePath)
		}
		return vault.Flatten(secrets), nil
	}

	// Check if it's a directory
	isDir, err := client.IsDirectory(ctx, basePath)
	if err != nil {
		// Not a directory error likely means it's a single secret
		isDir = false
	}

	if isDir {
		// Numeric version on directory doesn't make sense (versions are unrelated across secrets)
		if spec.Version > 0 {
			return nil, fmt.Errorf("numeric version (@%d) is only supported for single secrets, not directories\n  Each secret has its own independent version history.\n  Use: vlt diff %s/SECRET_NAME@%d %s/SECRET_NAME", spec.Version, basePath, spec.Version, basePath)
		}
		// @-N on directory: get state N changes ago
		if spec.IsChangesAgo {
			return client.GetStateAtChangesAgo(ctx, basePath, spec.ChangesAgo)
		}
		// @prev on directory: get previous version of each secret
		return client.GetPrevVersions(ctx, basePath)
	}

	// Single secret - @-N doesn't make sense for single secrets
	if spec.IsChangesAgo {
		return nil, fmt.Errorf("@-%d syntax is only supported for directories, not single secrets\n  Use @prev or @N for single secrets", spec.ChangesAgo)
	}

	// Single secret - get specific version
	secrets, err := client.GetSecretAtVersion(ctx, basePath, spec.Version, spec.IsPrev)
	if err != nil {
		return nil, err
	}
	// Flatten and extract values to match the format from regular Get path
	// forDirectory=false keeps standalone "value" key for single secrets
	return vault.FlattenAndExtractValues(secrets, false), nil
}

// getSecretsFromFile reads and parses a YAML file, returning a flat key->value map
func getSecretsFromFile(path string) (map[string]any, error) {
	var content []byte
	var err error

	if diffSops {
		content, err = decrypt.File(path, "yaml")
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt SOPS file: %w", err)
		}
	} else {
		content, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	}

	var data map[string]any
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Flatten nested structure to dot-notation keys
	return vault.Flatten(data), nil
}

func printDiffResult(path1, path2 string, result *vault.DiffResult) {
	if diffSummary {
		printSummary(result)
		return
	}

	fmt.Printf("Comparing %s → %s\n\n", path1, path2)

	if !result.HasDifferences() {
		fmt.Println("Paths are identical")
		return
	}

	if len(result.OnlyInFirst) > 0 {
		fmt.Printf("Only in %s:\n", path1)
		for _, dk := range result.OnlyInFirst {
			if diffShowValues {
				fmt.Printf("  - %s: %s\n", dk.Key, truncateValue(dk.Value))
			} else {
				fmt.Printf("  - %s\n", dk.Key)
			}
		}
		fmt.Println()
	}

	if len(result.OnlyInSecond) > 0 {
		fmt.Printf("Only in %s:\n", path2)
		for _, dk := range result.OnlyInSecond {
			if diffShowValues {
				fmt.Printf("  + %s: %s\n", dk.Key, truncateValue(dk.Value))
			} else {
				fmt.Printf("  + %s\n", dk.Key)
			}
		}
		fmt.Println()
	}

	if len(result.Changed) > 0 {
		fmt.Println("Changed:")
		for _, ck := range result.Changed {
			if diffShowValues {
				fmt.Printf("  ~ %s:\n", ck.Key)
				fmt.Printf("      - %s\n", truncateValue(ck.FirstValue))
				fmt.Printf("      + %s\n", truncateValue(ck.SecondValue))
			} else if diffKeysOnly {
				fmt.Printf("  ~ %s\n", ck.Key)
			} else {
				fmt.Printf("  ~ %s (%d → %d chars)\n", ck.Key, ck.FirstLen, ck.SecondLen)
			}
		}
		fmt.Println()
	}

	fmt.Printf("Unchanged: %d keys\n", result.Unchanged)
}

func printSummary(result *vault.DiffResult) {
	total := len(result.OnlyInFirst) + len(result.OnlyInSecond) + len(result.Changed) + result.Unchanged
	fmt.Printf("Total keys: %d\n", total)
	fmt.Printf("  Only in first:  %d\n", len(result.OnlyInFirst))
	fmt.Printf("  Only in second: %d\n", len(result.OnlyInSecond))
	fmt.Printf("  Changed:        %d\n", len(result.Changed))
	fmt.Printf("  Unchanged:      %d\n", result.Unchanged)
}

// truncateValue truncates long values for display
func truncateValue(s string) string {
	const maxLen = 80
	// Replace newlines with \n for single-line display
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
