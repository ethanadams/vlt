package vault

import (
	"context"
	"sort"
	"strings"
)

// TreeNode represents a node in the secret tree hierarchy
type TreeNode struct {
	Name     string
	FullPath string
	IsDir    bool
	Children []*TreeNode
	Metadata *SecretMetadata // Only populated if requested
}

// GetTree builds a tree structure of all secrets under a path
func (c *Client) GetTree(ctx context.Context, path string) (*TreeNode, error) {
	path = strings.TrimSuffix(path, "/")

	// Get all secret paths under this path
	secretPaths, err := c.ListSecretPaths(ctx, path)
	if err != nil {
		return nil, err
	}

	// Build the root node
	parts := strings.Split(path, "/")
	root := &TreeNode{
		Name:     parts[len(parts)-1] + "/",
		FullPath: path,
		IsDir:    true,
		Children: make([]*TreeNode, 0),
	}

	if len(secretPaths) == 0 {
		return root, nil
	}

	// Build tree from paths
	for _, relPath := range secretPaths {
		addPathToTree(root, path, relPath)
	}

	// Sort children at each level
	sortTree(root)

	return root, nil
}

// GetTreeWithMetadata builds a tree with metadata for each secret
func (c *Client) GetTreeWithMetadata(ctx context.Context, path string) (*TreeNode, error) {
	tree, err := c.GetTree(ctx, path)
	if err != nil {
		return nil, err
	}

	// Populate metadata for all leaf nodes
	c.populateMetadata(ctx, tree)

	return tree, nil
}

// addPathToTree adds a relative path to the tree structure
func addPathToTree(root *TreeNode, basePath, relPath string) {
	parts := strings.Split(relPath, "/")
	current := root

	for i, part := range parts {
		isLast := i == len(parts)-1

		// Look for existing child
		var found *TreeNode
		for _, child := range current.Children {
			childName := child.Name
			if child.IsDir {
				childName = strings.TrimSuffix(childName, "/")
			}
			if childName == part {
				found = child
				break
			}
		}

		if found == nil {
			// Create new node
			fullPath := basePath + "/" + strings.Join(parts[:i+1], "/")
			newNode := &TreeNode{
				Name:     part,
				FullPath: fullPath,
				IsDir:    !isLast,
				Children: make([]*TreeNode, 0),
			}
			if newNode.IsDir {
				newNode.Name = part + "/"
			}
			current.Children = append(current.Children, newNode)
			found = newNode
		}

		current = found
	}
}

// sortTree recursively sorts all children alphabetically (directories first)
func sortTree(node *TreeNode) {
	sort.Slice(node.Children, func(i, j int) bool {
		// Directories come first
		if node.Children[i].IsDir != node.Children[j].IsDir {
			return node.Children[i].IsDir
		}
		return node.Children[i].Name < node.Children[j].Name
	})

	for _, child := range node.Children {
		if child.IsDir {
			sortTree(child)
		}
	}
}

// populateMetadata adds metadata to all leaf nodes in the tree
func (c *Client) populateMetadata(ctx context.Context, node *TreeNode) {
	if !node.IsDir {
		// Leaf node - get metadata
		metadata, err := c.GetMetadata(ctx, node.FullPath)
		if err == nil {
			node.Metadata = metadata
		}
		return
	}

	// Recurse into children
	for _, child := range node.Children {
		c.populateMetadata(ctx, child)
	}
}

// Walk traverses the tree and calls the callback for each node
// The callback receives the node, depth, and whether it's the last child at its level
func (t *TreeNode) Walk(callback func(node *TreeNode, depth int, isLast bool)) {
	if t == nil {
		return
	}
	t.walkRecursive(callback, 0, true)
}

func (t *TreeNode) walkRecursive(callback func(node *TreeNode, depth int, isLast bool), depth int, isLast bool) {
	callback(t, depth, isLast)
	for i, child := range t.Children {
		child.walkRecursive(callback, depth+1, i == len(t.Children)-1)
	}
}

// CountSecrets returns the total number of secrets (non-directory nodes) in the tree
func (t *TreeNode) CountSecrets() int {
	if t == nil {
		return 0
	}
	if !t.IsDir {
		return 1
	}
	count := 0
	for _, child := range t.Children {
		count += child.CountSecrets()
	}
	return count
}

// CountDirs returns the total number of directories in the tree (excluding root)
func (t *TreeNode) CountDirs() int {
	if t == nil {
		return 0
	}
	count := 0
	for _, child := range t.Children {
		if child.IsDir {
			count += 1 + child.CountDirs()
		}
	}
	return count
}
