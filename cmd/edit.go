package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
If the path is a single secret, all keys in that secret are edited.

If no changes are detected, nothing is updated.

Example:
  vlt edit secret/myapp/db
  # Edit all keys in the db secret

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

	// Try to resolve the path to see if it's a secret
	resolved, err := client.ResolvePath(ctx, path)
	if err == nil {
		if resolved.Key != "" {
			return fmt.Errorf("cannot edit a single key (%s); edit the whole secret instead: %s", resolved.Key, resolved.SecretPath)
		}
		return runEditSingle(ctx, client, resolved.SecretPath)
	}

	// Path didn't resolve to a secret - check if it's a directory
	isDir, err := client.IsDirectory(ctx, path)
	if err != nil {
		return fmt.Errorf("path not found: %s", path)
	}

	if isDir {
		return runEditRecursive(ctx, client, path)
	}

	return fmt.Errorf("no secret or directory found at %s", path)
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

	// Import the modified structure back to Vault
	// This will create/update secrets based on the YAML structure
	count, err := client.Import(ctx, path, newSecrets)
	if err != nil {
		return fmt.Errorf("failed to write changes: %w", err)
	}

	if count == 1 {
		fmt.Printf("Updated 1 key.\n")
	} else {
		fmt.Printf("Updated %d keys.\n", count)
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
