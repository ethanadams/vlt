package vault

import (
	"testing"
)

func TestTreeNodeCountSecrets(t *testing.T) {
	tests := []struct {
		name     string
		tree     *TreeNode
		expected int
	}{
		{
			name:     "nil tree",
			tree:     nil,
			expected: 0,
		},
		{
			name: "single secret",
			tree: &TreeNode{
				Name:  "root",
				IsDir: true,
				Children: []*TreeNode{
					{Name: "secret1", IsDir: false},
				},
			},
			expected: 1,
		},
		{
			name: "nested secrets",
			tree: &TreeNode{
				Name:  "root",
				IsDir: true,
				Children: []*TreeNode{
					{Name: "secret1", IsDir: false},
					{
						Name:  "subdir",
						IsDir: true,
						Children: []*TreeNode{
							{Name: "secret2", IsDir: false},
							{Name: "secret3", IsDir: false},
						},
					},
				},
			},
			expected: 3,
		},
		{
			name: "only directories",
			tree: &TreeNode{
				Name:  "root",
				IsDir: true,
				Children: []*TreeNode{
					{Name: "subdir1", IsDir: true},
					{Name: "subdir2", IsDir: true},
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tree.CountSecrets(); got != tt.expected {
				t.Errorf("CountSecrets() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestTreeNodeCountDirs(t *testing.T) {
	tests := []struct {
		name     string
		tree     *TreeNode
		expected int
	}{
		{
			name:     "nil tree",
			tree:     nil,
			expected: 0,
		},
		{
			name: "root only",
			tree: &TreeNode{
				Name:  "root",
				IsDir: true,
			},
			expected: 0, // Root doesn't count
		},
		{
			name: "with subdirs",
			tree: &TreeNode{
				Name:  "root",
				IsDir: true,
				Children: []*TreeNode{
					{Name: "subdir1", IsDir: true},
					{Name: "subdir2", IsDir: true},
					{Name: "secret", IsDir: false},
				},
			},
			expected: 2,
		},
		{
			name: "nested dirs",
			tree: &TreeNode{
				Name:  "root",
				IsDir: true,
				Children: []*TreeNode{
					{
						Name:  "subdir1",
						IsDir: true,
						Children: []*TreeNode{
							{Name: "nested", IsDir: true},
						},
					},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tree.CountDirs(); got != tt.expected {
				t.Errorf("CountDirs() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestTreeNodeWalk(t *testing.T) {
	tree := &TreeNode{
		Name:  "root",
		IsDir: true,
		Children: []*TreeNode{
			{Name: "a", IsDir: false},
			{
				Name:  "b",
				IsDir: true,
				Children: []*TreeNode{
					{Name: "c", IsDir: false},
				},
			},
		},
	}

	var visited []string
	var depths []int

	tree.Walk(func(node *TreeNode, depth int, isLast bool) {
		visited = append(visited, node.Name)
		depths = append(depths, depth)
	})

	expectedNames := []string{"root", "a", "b", "c"}
	expectedDepths := []int{0, 1, 1, 2}

	if len(visited) != len(expectedNames) {
		t.Fatalf("visited %d nodes, expected %d", len(visited), len(expectedNames))
	}

	for i, name := range expectedNames {
		if visited[i] != name {
			t.Errorf("visited[%d] = %q, want %q", i, visited[i], name)
		}
		if depths[i] != expectedDepths[i] {
			t.Errorf("depth[%d] = %d, want %d", i, depths[i], expectedDepths[i])
		}
	}
}

func TestTreeNodeWalkNil(t *testing.T) {
	var tree *TreeNode
	called := false

	tree.Walk(func(node *TreeNode, depth int, isLast bool) {
		called = true
	})

	if called {
		t.Error("Walk should not call callback for nil tree")
	}
}
