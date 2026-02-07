package cmd

import (
	"context"
	"fmt"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var findRecursive bool

var findCmd = &cobra.Command{
	Use:   "find <path> <pattern>",
	Short: "Find keys matching a pattern",
	Long: `Find keys matching a glob pattern at the given path.

Without -r, only searches secrets at the immediate level.
With -r, recursively searches all secrets under the path.

Pattern supports glob syntax: *, ?, [abc], [a-z].

Example:
  vlt find secret/myapp "pass*"
  # Finds keys starting with "pass" in secrets at secret/myapp

  vlt find secret/myapp "*" -r
  # Lists all keys recursively under secret/myapp

  vlt find secret/myapp "db.*" -r
  # Finds keys starting with "db." recursively`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFind(cmd.Context(), args[0], args[1])
	},
}

func init() {
	findCmd.Flags().BoolVarP(&findRecursive, "recursive", "r", false, "search recursively")
	rootCmd.AddCommand(findCmd)
}

func runFind(ctx context.Context, path, pattern string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	results, err := client.FindKeys(ctx, path, pattern, findRecursive)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		return fmt.Errorf("no keys matching %q found at %s", pattern, path)
	}

	if isStructuredOutput() {
		type findResult struct {
			Path string `json:"path" yaml:"path"`
		}
		var out []findResult
		for _, r := range results {
			out = append(out, findResult{Path: r.FullPath()})
		}
		return printOutput(out)
	}

	for _, r := range results {
		fmt.Println(r.FullPath())
	}

	return nil
}
