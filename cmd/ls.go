package cmd

import (
	"context"
	"fmt"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var lsLong bool

var lsCmd = &cobra.Command{
	Use:   "ls <path>",
	Short: "List secrets and directories",
	Long: `List secrets and directories at the given path.

Use -l for detailed output including metadata.

Example:
  vlt ls secret/myapp

  vlt ls secret/myapp -l`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLs(cmd.Context(), args[0])
	},
}

func init() {
	lsCmd.Flags().BoolVarP(&lsLong, "long", "l", false, "show detailed metadata")
	rootCmd.AddCommand(lsCmd)
}

func runLs(ctx context.Context, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
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

	for _, entry := range entries {
		if lsLong {
			if entry.IsDir {
				fmt.Printf("d  %-40s\n", entry.Name+"/")
			} else {
				fmt.Printf("s  %-40s v%-4d %s\n", entry.Name, entry.Version, entry.UpdatedAt)
			}
		} else {
			if entry.IsDir {
				fmt.Printf("%s/\n", entry.Name)
			} else {
				fmt.Println(entry.Name)
			}
		}
	}

	return nil
}
