package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var editCmd = &cobra.Command{
	Use:   "edit <path>",
	Short: "Edit secrets in your editor",
	Long: `Edit secrets in your default editor (like kubectl edit).

Opens the secret(s) at the given path in $EDITOR (or $VISUAL, or vi).
After you save and close the editor, changes are written back to Vault.

If the path is a directory, all secrets under it are loaded for editing.
If the path is a single secret, only that secret is edited.

If no changes are detected, nothing is updated.

Example:
  vlt edit secret/myapp/config
  # Edit a single secret

  vlt edit secret/myapp
  # Edit all secrets under myapp

  EDITOR=nano vlt edit secret/myapp
  # Use nano as the editor`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEdit(cmd.Context(), args[0])
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
}

func runEdit(ctx context.Context, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	// Check if path is a directory (has children)
	isDir, err := client.IsDirectory(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to check path: %w", err)
	}

	if isDir {
		return runEditRecursive(ctx, client, path)
	}
	return runEditSingle(ctx, client, path)
}

func runEditSingle(ctx context.Context, client *vault.Client, path string) error {
	// Read current secret
	data, err := client.ReadSecretRaw(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to read secret: %w", err)
	}

	if data == nil {
		return fmt.Errorf("secret not found at %s", path)
	}

	// Convert to YAML
	originalYAML, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal secret: %w", err)
	}

	// Open in editor
	modifiedYAML, err := openInEditor(originalYAML)
	if err != nil {
		return err
	}

	// Check if content changed
	if hashBytes(originalYAML) == hashBytes(modifiedYAML) {
		fmt.Println("Edit cancelled, no changes made.")
		return nil
	}

	// Parse modified YAML
	var newData map[string]any
	if err := yaml.Unmarshal(modifiedYAML, &newData); err != nil {
		return fmt.Errorf("failed to parse modified YAML: %w", err)
	}

	// Write back to Vault
	if err := client.WriteSecret(ctx, path, newData); err != nil {
		return fmt.Errorf("failed to write secret: %w", err)
	}

	fmt.Printf("Secret %s updated.\n", path)
	return nil
}

func runEditRecursive(ctx context.Context, client *vault.Client, path string) error {
	// Get all secrets recursively
	secrets, err := client.Get(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to read secrets: %w", err)
	}

	if len(secrets) == 0 {
		return fmt.Errorf("no secrets found at %s", path)
	}

	// Flatten for comparison later
	originalFlat := vault.Flatten(secrets)

	// Convert to YAML
	originalYAML, err := yaml.Marshal(secrets)
	if err != nil {
		return fmt.Errorf("failed to marshal secrets: %w", err)
	}

	// Open in editor
	modifiedYAML, err := openInEditor(originalYAML)
	if err != nil {
		return err
	}

	// Check if content changed
	if hashBytes(originalYAML) == hashBytes(modifiedYAML) {
		fmt.Println("Edit cancelled, no changes made.")
		return nil
	}

	// Parse modified YAML
	var newSecrets map[string]any
	if err := yaml.Unmarshal(modifiedYAML, &newSecrets); err != nil {
		return fmt.Errorf("failed to parse modified YAML: %w", err)
	}

	// Flatten modified secrets
	modifiedFlat := vault.Flatten(newSecrets)

	// Find changes
	var added, changed, removed []string

	for key := range modifiedFlat {
		if _, exists := originalFlat[key]; !exists {
			added = append(added, key)
		} else if fmt.Sprintf("%v", modifiedFlat[key]) != fmt.Sprintf("%v", originalFlat[key]) {
			changed = append(changed, key)
		}
	}

	for key := range originalFlat {
		if _, exists := modifiedFlat[key]; !exists {
			removed = append(removed, key)
		}
	}

	// Sort for consistent output
	sort.Strings(added)
	sort.Strings(changed)
	sort.Strings(removed)

	if len(added) == 0 && len(changed) == 0 && len(removed) == 0 {
		fmt.Println("Edit cancelled, no changes made.")
		return nil
	}

	// Write changes to Vault
	writeCount := 0
	for _, key := range added {
		secretPath := path + "/" + key
		if err := client.Add(ctx, secretPath, fmt.Sprintf("%v", modifiedFlat[key])); err != nil {
			return fmt.Errorf("failed to add %s: %w", key, err)
		}
		fmt.Printf("  + %s\n", key)
		writeCount++
	}

	for _, key := range changed {
		secretPath := path + "/" + key
		if err := client.Update(ctx, secretPath, fmt.Sprintf("%v", modifiedFlat[key])); err != nil {
			return fmt.Errorf("failed to update %s: %w", key, err)
		}
		fmt.Printf("  ~ %s\n", key)
		writeCount++
	}

	// Delete removed keys
	deleteCount := 0
	for _, key := range removed {
		secretPath := path + "/" + key
		if err := client.DeleteSecret(ctx, secretPath); err != nil {
			return fmt.Errorf("failed to delete %s: %w", key, err)
		}
		fmt.Printf("  - %s\n", key)
		deleteCount++
	}

	total := writeCount + deleteCount
	if total == 1 {
		fmt.Printf("\nUpdated 1 secret.\n")
	} else {
		fmt.Printf("\nUpdated %d secrets.\n", total)
	}
	return nil
}

func openInEditor(content []byte) ([]byte, error) {
	// Create temp file with restrictive permissions (secrets!)
	tmpFile, err := os.CreateTemp("", "vlt-edit-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Set restrictive permissions before writing secrets
	if err := os.Chmod(tmpPath, 0600); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("failed to set temp file permissions: %w", err)
	}

	// Write content
	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	// Get editor
	editor := getEditor()

	// Open editor
	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("editor failed: %w", err)
	}

	// Read modified content
	return os.ReadFile(tmpPath)
}

// getEditor returns the editor to use, checking $EDITOR, $VISUAL, then common defaults
func getEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if editor := os.Getenv("VISUAL"); editor != "" {
		return editor
	}

	// Try common editors
	editors := []string{"vim", "vi", "nano", "notepad"}
	for _, editor := range editors {
		if path, err := exec.LookPath(editor); err == nil {
			return filepath.Base(path)
		}
	}

	return "vi" // fallback
}

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h[:])
}
