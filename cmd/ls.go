package cmd

import (
	"context"
	"fmt"
	"sort"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var (
	lsLong bool
	lsKeys bool
)

var lsCmd = &cobra.Command{
	Use:     "ls <path>",
	Aliases: []string{"list"},
	Short:   "List secrets and directories",
	Long: `List secrets and directories at the given path.

Use -l for detailed output including metadata.
Use -k to list keys within a secret.

Example:
  vlt ls secret/myapp
  # Lists secrets and directories under myapp

  vlt ls secret/myapp -l
  # Lists with metadata (version, timestamp)

  vlt ls secret/myapp/db -k
  # Lists keys within the db secret`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLs(cmd.Context(), args[0])
	},
}

func init() {
	lsCmd.Flags().BoolVarP(&lsLong, "long", "l", false, "show detailed metadata")
	lsCmd.Flags().BoolVarP(&lsKeys, "keys", "k", false, "list keys within a secret")
	rootCmd.AddCommand(lsCmd)
}

func runLs(ctx context.Context, path string) error {
	initColor()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	// If -k flag, list keys within a secret
	if lsKeys {
		return listKeys(ctx, client, path)
	}

	var entries []vault.ListEntry
	if lsLong {
		entries, err = client.ListWithMetadata(ctx, path)
	} else {
		entries, err = client.List(ctx, path)
	}
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return fmt.Errorf("no secrets or directories found at %s", path)
	}

	if isStructuredOutput() {
		type lsEntry struct {
			Name      string `json:"name" yaml:"name"`
			Type      string `json:"type" yaml:"type"`
			Version   int    `json:"version,omitempty" yaml:"version,omitempty"`
			UpdatedAt string `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
		}
		var out []lsEntry
		for _, entry := range entries {
			e := lsEntry{Name: entry.Name, Type: "secret"}
			if entry.IsDir {
				e.Type = "directory"
			} else {
				e.Version = entry.Version
				e.UpdatedAt = entry.UpdatedAt
			}
			out = append(out, e)
		}
		return printOutput(out)
	}

	for _, entry := range entries {
		if lsLong {
			if entry.IsDir {
				fmt.Printf("d  %-40s\n", colorCyan(entry.Name+"/"))
			} else {
				fmt.Printf("s  %-40s v%-4d %s\n", entry.Name, entry.Version, entry.UpdatedAt)
			}
		} else {
			if entry.IsDir {
				fmt.Printf("%s\n", colorCyan(entry.Name+"/"))
			} else {
				fmt.Println(entry.Name)
			}
		}
	}

	return nil
}

func listKeys(ctx context.Context, client *vault.Client, path string) error {
	// Try to resolve the path to a secret
	resolved, err := client.ResolvePath(ctx, path)
	if err != nil {
		return fmt.Errorf("no secret found at %s", path)
	}

	if resolved.Key != "" {
		return fmt.Errorf("%s is a key, not a secret", path)
	}

	data, err := client.ReadSecretRaw(ctx, resolved.SecretPath)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return fmt.Errorf("no keys found in secret at %s", path)
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		fmt.Println(key)
	}

	return nil
}
