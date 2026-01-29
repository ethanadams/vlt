package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/spf13/cobra"
)

var (
	treeLong bool
)

var treeCmd = &cobra.Command{
	Use:   "tree <path>",
	Short: "Display secrets in a tree view",
	Long: `Display the secret hierarchy under a path as a tree.

Shows directories and secrets in a visual tree format.
Use -l to include version and timestamp metadata.

Examples:
  vlt tree secret/myapp
  vlt tree secret/myapp -l    # include metadata`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTree(cmd.Context(), args[0])
	},
}

func init() {
	treeCmd.Flags().BoolVarP(&treeLong, "long", "l", false, "show version and timestamp metadata")
	rootCmd.AddCommand(treeCmd)
}

func runTree(ctx context.Context, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return err
	}

	var tree *vault.TreeNode
	if treeLong {
		tree, err = client.GetTreeWithMetadata(ctx, path)
	} else {
		tree, err = client.GetTree(ctx, path)
	}
	if err != nil {
		return err
	}

	printTree(tree, treeLong)

	// Print summary
	secrets := tree.CountSecrets()
	dirs := tree.CountDirs()
	fmt.Printf("\n%d directories, %d secrets\n", dirs, secrets)

	return nil
}

// printTree prints the tree with box-drawing characters
func printTree(tree *vault.TreeNode, showMetadata bool) {
	// Track the prefix for each level
	var prefixes []string

	tree.Walk(func(node *vault.TreeNode, depth int, isLast bool) {
		if depth == 0 {
			// Root node
			fmt.Println(node.Name)
			prefixes = []string{}
			return
		}

		// Build the prefix
		var prefix strings.Builder
		for i := 0; i < depth-1; i++ {
			if i < len(prefixes) {
				prefix.WriteString(prefixes[i])
			}
		}

		// Add the connector
		if isLast {
			prefix.WriteString("└── ")
		} else {
			prefix.WriteString("├── ")
		}

		// Print the node
		if showMetadata && node.Metadata != nil {
			fmt.Printf("%s%s  v%d  %s\n",
				prefix.String(),
				node.Name,
				node.Metadata.CurrentVersion,
				node.Metadata.UpdatedTime.Local().Format("2006-01-02 15:04:05"),
			)
		} else {
			fmt.Printf("%s%s\n", prefix.String(), node.Name)
		}

		// Update prefixes for children
		for len(prefixes) < depth {
			prefixes = append(prefixes, "")
		}
		if isLast {
			prefixes[depth-1] = "    "
		} else {
			prefixes[depth-1] = "│   "
		}
	})
}
