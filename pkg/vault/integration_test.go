//go:build integration

package vault_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ethanadams/vlt/pkg/config"
	"github.com/ethanadams/vlt/pkg/vault"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const testToken = "test-root-token"

// vaultContainer holds the running Vault container
type vaultContainer struct {
	testcontainers.Container
	URI string
}

// setupVault starts a Vault container for testing
func setupVault(ctx context.Context) (*vaultContainer, error) {
	req := testcontainers.ContainerRequest{
		Image:        "hashicorp/vault:latest",
		ExposedPorts: []string{"8200/tcp"},
		Env: map[string]string{
			"VAULT_DEV_ROOT_TOKEN_ID":    testToken,
			"VAULT_DEV_LISTEN_ADDRESS":   "0.0.0.0:8200",
			"VAULT_ADDR":                 "http://0.0.0.0:8200",
		},
		Cmd: []string{"server", "-dev"},
		WaitingFor: wait.ForAll(
			wait.ForHTTP("/v1/sys/health").WithPort("8200/tcp"),
			wait.ForLog("Development mode"),
		).WithDeadline(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start vault container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "8200/tcp")
	if err != nil {
		return nil, fmt.Errorf("failed to get container port: %w", err)
	}

	return &vaultContainer{
		Container: container,
		URI:       fmt.Sprintf("http://%s:%s", host, port.Port()),
	}, nil
}

// newTestClient creates a vault client connected to the test container
func newTestClient(uri string) (*vault.Client, error) {
	cfg := &config.Config{
		VaultAddr:  uri,
		VaultToken: testToken,
	}
	return vault.NewClient(cfg)
}

func TestIntegration_AddGetSecret(t *testing.T) {
	ctx := context.Background()

	container, err := setupVault(ctx)
	if err != nil {
		t.Fatalf("failed to setup vault: %v", err)
	}
	defer container.Terminate(ctx)

	client, err := newTestClient(container.URI)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Test Add
	err = client.Add(ctx, "secret/test/mykey", "myvalue")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Test Get
	secrets, err := client.Get(ctx, "secret/test/mykey")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if secrets["value"] != "myvalue" {
		t.Errorf("expected value 'myvalue', got %v", secrets["value"])
	}
}

func TestIntegration_UpdateSecret(t *testing.T) {
	ctx := context.Background()

	container, err := setupVault(ctx)
	if err != nil {
		t.Fatalf("failed to setup vault: %v", err)
	}
	defer container.Terminate(ctx)

	client, err := newTestClient(container.URI)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Add initial secret
	err = client.Add(ctx, "secret/test/update", "initial")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Update it
	err = client.Update(ctx, "secret/test/update", "updated")
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify
	secrets, err := client.Get(ctx, "secret/test/update")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if secrets["value"] != "updated" {
		t.Errorf("expected 'updated', got %v", secrets["value"])
	}
}

func TestIntegration_DeleteSecret(t *testing.T) {
	ctx := context.Background()

	container, err := setupVault(ctx)
	if err != nil {
		t.Fatalf("failed to setup vault: %v", err)
	}
	defer container.Terminate(ctx)

	client, err := newTestClient(container.URI)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Add and delete
	_ = client.Add(ctx, "secret/test/delete", "value")
	err = client.DeleteSecret(ctx, "secret/test/delete")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	exists, err := client.SecretExists(ctx, "secret/test/delete")
	if err != nil {
		t.Fatalf("SecretExists failed: %v", err)
	}
	if exists {
		t.Error("secret should not exist after delete")
	}
}

func TestIntegration_CopySecret(t *testing.T) {
	ctx := context.Background()

	container, err := setupVault(ctx)
	if err != nil {
		t.Fatalf("failed to setup vault: %v", err)
	}
	defer container.Terminate(ctx)

	client, err := newTestClient(container.URI)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Add source
	_ = client.Add(ctx, "secret/test/src", "copyvalue")

	// Copy
	err = client.Copy(ctx, "secret/test/src", "secret/test/dst")
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	// Verify destination
	secrets, err := client.Get(ctx, "secret/test/dst")
	if err != nil {
		t.Fatalf("Get destination failed: %v", err)
	}

	if secrets["value"] != "copyvalue" {
		t.Errorf("expected 'copyvalue', got %v", secrets["value"])
	}

	// Verify source still exists
	srcSecrets, err := client.Get(ctx, "secret/test/src")
	if err != nil {
		t.Fatalf("Get source failed: %v", err)
	}
	if srcSecrets["value"] != "copyvalue" {
		t.Error("source should still exist after copy")
	}
}

func TestIntegration_MoveSecret(t *testing.T) {
	ctx := context.Background()

	container, err := setupVault(ctx)
	if err != nil {
		t.Fatalf("failed to setup vault: %v", err)
	}
	defer container.Terminate(ctx)

	client, err := newTestClient(container.URI)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Add source
	_ = client.Add(ctx, "secret/test/move-src", "movevalue")

	// Move
	err = client.Move(ctx, "secret/test/move-src", "secret/test/move-dst")
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	// Verify destination
	secrets, err := client.Get(ctx, "secret/test/move-dst")
	if err != nil {
		t.Fatalf("Get destination failed: %v", err)
	}

	if secrets["value"] != "movevalue" {
		t.Errorf("expected 'movevalue', got %v", secrets["value"])
	}

	// Verify source is gone
	exists, _ := client.SecretExists(ctx, "secret/test/move-src")
	if exists {
		t.Error("source should not exist after move")
	}
}

func TestIntegration_ListSecrets(t *testing.T) {
	ctx := context.Background()

	container, err := setupVault(ctx)
	if err != nil {
		t.Fatalf("failed to setup vault: %v", err)
	}
	defer container.Terminate(ctx)

	client, err := newTestClient(container.URI)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Add multiple secrets
	_ = client.Add(ctx, "secret/list/key1", "value1")
	_ = client.Add(ctx, "secret/list/key2", "value2")
	_ = client.Add(ctx, "secret/list/sub/key3", "value3")

	// List
	entries, err := client.List(ctx, "secret/list")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Check for expected entries
	found := make(map[string]bool)
	for _, e := range entries {
		found[e.Name] = true
	}

	for _, expected := range []string{"key1", "key2", "sub"} {
		if !found[expected] {
			t.Errorf("expected entry %q not found", expected)
		}
	}
}

func TestIntegration_VersionHistory(t *testing.T) {
	ctx := context.Background()

	container, err := setupVault(ctx)
	if err != nil {
		t.Fatalf("failed to setup vault: %v", err)
	}
	defer container.Terminate(ctx)

	client, err := newTestClient(container.URI)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Create secret with multiple versions
	_ = client.Add(ctx, "secret/version/test", "v1")
	_ = client.Update(ctx, "secret/version/test", "v2")
	_ = client.Update(ctx, "secret/version/test", "v3")

	// Get version history
	history, err := client.GetVersionHistory(ctx, "secret/version/test")
	if err != nil {
		t.Fatalf("GetVersionHistory failed: %v", err)
	}

	if len(history) != 3 {
		t.Errorf("expected 3 versions, got %d", len(history))
	}

	// Should be sorted descending (newest first)
	if history[0].Version != 3 {
		t.Errorf("expected newest version 3, got %d", history[0].Version)
	}
}

func TestIntegration_ReadSpecificVersion(t *testing.T) {
	ctx := context.Background()

	container, err := setupVault(ctx)
	if err != nil {
		t.Fatalf("failed to setup vault: %v", err)
	}
	defer container.Terminate(ctx)

	client, err := newTestClient(container.URI)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Create secret with multiple versions
	_ = client.Add(ctx, "secret/readver/test", "version-one")
	_ = client.Update(ctx, "secret/readver/test", "version-two")

	// Read version 1
	v1Data, err := client.ReadSecretVersion(ctx, "secret/readver/test", 1)
	if err != nil {
		t.Fatalf("ReadSecretVersion failed: %v", err)
	}

	if v1Data["value"] != "version-one" {
		t.Errorf("expected 'version-one', got %v", v1Data["value"])
	}

	// Read version 2
	v2Data, err := client.ReadSecretVersion(ctx, "secret/readver/test", 2)
	if err != nil {
		t.Fatalf("ReadSecretVersion v2 failed: %v", err)
	}

	if v2Data["value"] != "version-two" {
		t.Errorf("expected 'version-two', got %v", v2Data["value"])
	}
}

func TestIntegration_Snapshot(t *testing.T) {
	ctx := context.Background()

	container, err := setupVault(ctx)
	if err != nil {
		t.Fatalf("failed to setup vault: %v", err)
	}
	defer container.Terminate(ctx)

	client, err := newTestClient(container.URI)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Create secrets
	_ = client.Add(ctx, "secret/snap/key1", "value1")
	_ = client.Add(ctx, "secret/snap/key2", "value2")

	// Create snapshot
	snapshot, err := client.CreateSnapshot(ctx, "secret/snap")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if len(snapshot.Secrets) != 2 {
		t.Errorf("expected 2 secrets in snapshot, got %d", len(snapshot.Secrets))
	}

	if snapshot.Path != "secret/snap" {
		t.Errorf("expected path 'secret/snap', got %s", snapshot.Path)
	}
}

func TestIntegration_Restore(t *testing.T) {
	ctx := context.Background()

	container, err := setupVault(ctx)
	if err != nil {
		t.Fatalf("failed to setup vault: %v", err)
	}
	defer container.Terminate(ctx)

	client, err := newTestClient(container.URI)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Create secrets and snapshot
	_ = client.Add(ctx, "secret/restore/key1", "original")
	snapshot, _ := client.CreateSnapshot(ctx, "secret/restore")

	// Modify the secret
	_ = client.Update(ctx, "secret/restore/key1", "modified")

	// Restore
	result, err := client.RestoreSnapshot(ctx, snapshot, "secret/restore", vault.RestoreOptions{})
	if err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}

	if len(result.Updated) != 1 {
		t.Errorf("expected 1 updated, got %d", len(result.Updated))
	}

	// Verify restored value
	secrets, _ := client.Get(ctx, "secret/restore/key1")
	if secrets["value"] != "original" {
		t.Errorf("expected 'original', got %v", secrets["value"])
	}
}

func TestIntegration_FindDuplicates(t *testing.T) {
	ctx := context.Background()

	container, err := setupVault(ctx)
	if err != nil {
		t.Fatalf("failed to setup vault: %v", err)
	}
	defer container.Terminate(ctx)

	client, err := newTestClient(container.URI)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Create secrets with duplicate values
	_ = client.Add(ctx, "secret/dup/key1", "same-value")
	_ = client.Add(ctx, "secret/dup/key2", "same-value")
	_ = client.Add(ctx, "secret/dup/key3", "different")

	// Find duplicates
	duplicates, err := client.FindDuplicates(ctx, "secret/dup")
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}

	// Should have one set of duplicates (key1 and key2)
	if len(duplicates) != 1 {
		t.Errorf("expected 1 duplicate group, got %d", len(duplicates))
	}

	if len(duplicates) > 0 && len(duplicates[0].Paths) != 2 {
		t.Errorf("expected 2 paths in duplicate group, got %d", len(duplicates[0].Paths))
	}
}
