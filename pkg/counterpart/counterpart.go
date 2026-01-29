// Package counterpart provides functionality to update companion YAML files
// with Vault references after secrets are imported.
package counterpart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DeriveFilename derives the counterpart filename from a source file path.
// Examples:
//   - "app-secrets.enc.yaml" -> "app.yaml"
//   - "/path/to/config-secrets.yaml" -> "/path/to/config.yaml"
func DeriveFilename(sourcePath string) string {
	dir := filepath.Dir(sourcePath)
	name := CleanFilename(sourcePath)
	return filepath.Join(dir, name+".yaml")
}

// CleanFilename extracts a clean name from a filename.
// Examples:
//   - "app-secrets.enc.yaml" -> "app"
//   - "myapp.sops.yaml" -> "myapp"
//   - "/path/to/config-secrets.yaml" -> "config"
func CleanFilename(path string) string {
	// Get base filename without directory
	name := filepath.Base(path)

	// Strip "-secrets" and everything after
	if idx := strings.Index(name, "-secrets"); idx != -1 {
		return name[:idx]
	}

	// Otherwise strip from first "."
	if idx := strings.Index(name, "."); idx != -1 {
		return name[:idx]
	}

	return name
}

// UpdateResult contains information about a counterpart update operation
type UpdateResult struct {
	Updated bool
	Path    string
	Keys    int
}

// Update updates a counterpart YAML file with vault references.
// For each key in keys, it sets the value to ref+vault://<vaultPath>/<key>#value.
// If the key exists nested in the counterpart, it updates nested. Otherwise adds as flat key.
// Only updates if the file exists. Preserves original formatting and indentation.
func Update(path, vaultPath string, keys []string) (*UpdateResult, error) {
	result := &UpdateResult{Path: path}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return result, nil // File doesn't exist, skip silently
	}

	// Read existing file
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	// Detect original indentation (default to 2)
	indent := detectIndent(content)

	// Parse YAML into Node to preserve ordering
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	// Find the root mapping node
	var root *yaml.Node
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		root = doc.Content[0]
	} else if doc.Kind == yaml.MappingNode {
		root = &doc
	}

	if root == nil || root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected YAML mapping at root, got kind %v", doc.Kind)
	}

	// Update or add each key
	for _, key := range keys {
		vaultRef := fmt.Sprintf("ref+vault://%s/%s#value", vaultPath, key)
		keyPath := strings.Split(key, ".")

		// Try to find and update the key, or add at deepest matching path
		upsertNestedKey(root, keyPath, vaultRef)
	}

	// Write back with original indentation
	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(indent)
	if err := encoder.Encode(&doc); err != nil {
		return nil, fmt.Errorf("marshaling YAML: %w", err)
	}
	encoder.Close()

	if err := os.WriteFile(path, []byte(buf.String()), 0644); err != nil {
		return nil, fmt.Errorf("writing file: %w", err)
	}

	result.Updated = true
	result.Keys = len(keys)
	return result, nil
}

// FormatRef formats a vault reference string for a given path and key.
func FormatRef(vaultPath, key string) string {
	return fmt.Sprintf("ref+vault://%s/%s#value", vaultPath, key)
}

// detectIndent detects the indentation used in YAML content.
func detectIndent(content []byte) int {
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if len(trimmed) > 0 && len(trimmed) < len(line) {
			indent := len(line) - len(trimmed)
			if indent > 0 {
				return indent
			}
		}
	}
	return 2 // default
}

// upsertNestedKey finds the deepest matching nested path and either updates
// an existing key or adds a new one at the appropriate level.
// If the current level has flat keys (keys with dots), adds as flat key.
// Otherwise, creates nested structure.
func upsertNestedKey(node *yaml.Node, keyPath []string, value string) {
	if node.Kind != yaml.MappingNode || len(keyPath) == 0 {
		return
	}

	// First, try to find an exact match for the full flattened key at this level
	flatKey := strings.Join(keyPath, ".")
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == flatKey {
			// Found exact flat key match, update it
			node.Content[i+1].Value = value
			node.Content[i+1].Kind = yaml.ScalarNode
			node.Content[i+1].Tag = ""
			node.Content[i+1].Content = nil
			return
		}
	}

	// Try to find the first path segment as a nested mapping
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == keyPath[0] {
			if len(keyPath) == 1 {
				// Found the leaf key, update its value
				node.Content[i+1].Value = value
				node.Content[i+1].Kind = yaml.ScalarNode
				node.Content[i+1].Tag = ""
				node.Content[i+1].Content = nil
				return
			}
			// More path segments - if this is a mapping, recurse
			if node.Content[i+1].Kind == yaml.MappingNode {
				upsertNestedKey(node.Content[i+1], keyPath[1:], value)
				return
			}
			// Not a mapping, can't go deeper - shouldn't happen for well-formed data
			return
		}
	}

	// Key not found at this level
	// Check if this level has any flat keys (keys containing dots)
	if hasFlatKeys(node) {
		// Add as flat key
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: flatKey},
			&yaml.Node{Kind: yaml.ScalarNode, Value: value},
		)
	} else {
		// Create nested structure
		addNestedKey(node, keyPath, value)
	}
}

// hasFlatKeys checks if a mapping node has any keys containing dots
func hasFlatKeys(node *yaml.Node) bool {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if strings.Contains(node.Content[i].Value, ".") {
			return true
		}
	}
	return false
}

// addNestedKey creates nested structure for the key path
func addNestedKey(node *yaml.Node, keyPath []string, value string) {
	if len(keyPath) == 0 {
		return
	}

	if len(keyPath) == 1 {
		// Leaf node - add scalar value
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: keyPath[0]},
			&yaml.Node{Kind: yaml.ScalarNode, Value: value},
		)
		return
	}

	// Create nested mapping
	newMapping := &yaml.Node{Kind: yaml.MappingNode}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: keyPath[0]},
		newMapping,
	)
	addNestedKey(newMapping, keyPath[1:], value)
}
