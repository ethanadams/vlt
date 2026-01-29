package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	snapshotOutput string
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot <path>",
	Short: "Create a point-in-time backup of secrets",
	Long: `Create a snapshot of all secrets under a path.

The snapshot includes secret values, version numbers, and timestamps.
Use 'vlt restore' to restore secrets from a snapshot.

Examples:
  vlt snapshot secret/myapp -o backup.yaml
  vlt snapshot secret/myapp -o backup-$(date +%Y%m%d).yaml`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSnapshot(cmd.Context(), args[0])
	},
}

func init() {
	snapshotCmd.Flags().StringVarP(&snapshotOutput, "output", "o", "", "output file path (required)")
	_ = snapshotCmd.MarkFlagRequired("output")
	rootCmd.AddCommand(snapshotCmd)
}

func runSnapshot(ctx context.Context, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	// Create snapshot
	snapshot, err := client.CreateSnapshot(ctx, path)
	if err != nil {
		return err
	}

	// Marshal to YAML
	data, err := yaml.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	// Write to file
	if err := os.WriteFile(snapshotOutput, data, 0600); err != nil {
		return fmt.Errorf("failed to write snapshot file: %w", err)
	}

	fmt.Printf("Snapshot created: %s\n", snapshotOutput)
	fmt.Printf("  Path: %s\n", snapshot.Path)
	fmt.Printf("  Secrets: %d\n", len(snapshot.Secrets))
	fmt.Printf("  Created: %s\n", snapshot.CreatedAt.Local().Format("2006-01-02 15:04:05"))

	return nil
}

// LoadSnapshot loads a snapshot from a YAML file
func LoadSnapshot(path string) (*vault.Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot file: %w", err)
	}

	var snapshot vault.Snapshot
	if err := yaml.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to parse snapshot file: %w", err)
	}

	return &snapshot, nil
}
