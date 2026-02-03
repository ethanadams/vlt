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

	// Test Add - creates secret at secret/test with key "mykey"
	err = client.Add(ctx, "secret/test/mykey", "myvalue")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Test Get - returns the key and its value
	secrets, err := client.Get(ctx, "secret/test/mykey")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if secrets["mykey"] != "myvalue" {
		t.Errorf("expected value 'myvalue', got %v", secrets["mykey"])
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

	// Add initial secret - creates secret at secret/test with key "upkey"
	err = client.Add(ctx, "secret/test/upkey", "initial")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Update it
	err = client.Update(ctx, "secret/test/upkey", "updated")
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify
	secrets, err := client.Get(ctx, "secret/test/upkey")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if secrets["upkey"] != "updated" {
		t.Errorf("expected 'updated', got %v", secrets["upkey"])
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

	// Add a secret at secret/deltest with key "mykey"
	if err := client.Add(ctx, "secret/deltest/mykey", "value"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Delete the whole secret
	err = client.DeleteSecret(ctx, "secret/deltest")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	exists, err := client.SecretExists(ctx, "secret/deltest")
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

	// Add source key - creates secret at secret/copysrc with key "mykey"
	if err := client.Add(ctx, "secret/copysrc/mykey", "copyvalue"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Copy key to another secret
	err = client.Copy(ctx, "secret/copysrc/mykey", "secret/copydst/mykey")
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	// Verify destination
	secrets, err := client.Get(ctx, "secret/copydst/mykey")
	if err != nil {
		t.Fatalf("Get destination failed: %v", err)
	}

	if secrets["mykey"] != "copyvalue" {
		t.Errorf("expected 'copyvalue', got %v", secrets["mykey"])
	}

	// Verify source still exists
	srcSecrets, err := client.Get(ctx, "secret/copysrc/mykey")
	if err != nil {
		t.Fatalf("Get source failed: %v", err)
	}
	if srcSecrets["mykey"] != "copyvalue" {
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

	// Add source key - creates secret at secret/movesrc with key "mykey"
	if err := client.Add(ctx, "secret/movesrc/mykey", "movevalue"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Move key to another secret
	err = client.Move(ctx, "secret/movesrc/mykey", "secret/movedst/mykey")
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	// Verify destination
	secrets, err := client.Get(ctx, "secret/movedst/mykey")
	if err != nil {
		t.Fatalf("Get destination failed: %v", err)
	}

	if secrets["mykey"] != "movevalue" {
		t.Errorf("expected 'movevalue', got %v", secrets["mykey"])
	}

	// Verify source key is gone (the key was the only one, so secret should be empty or gone)
	exists, _ := client.KeyExists(ctx, "secret/movesrc", "mykey")
	if exists {
		t.Error("source key should not exist after move")
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

	// Create secrets at different paths under secret/list
	// secret/list/app1 with key "key"
	// secret/list/app2 with key "key"
	// secret/list/sub/app3 with key "key"
	if err := client.Add(ctx, "secret/list/app1/key", "value1"); err != nil {
		t.Fatalf("Add app1 failed: %v", err)
	}
	if err := client.Add(ctx, "secret/list/app2/key", "value2"); err != nil {
		t.Fatalf("Add app2 failed: %v", err)
	}
	if err := client.Add(ctx, "secret/list/sub/app3/key", "value3"); err != nil {
		t.Fatalf("Add app3 failed: %v", err)
	}

	// List at secret/list - should see app1, app2, and sub directory
	entries, err := client.List(ctx, "secret/list")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d: %+v", len(entries), entries)
	}

	// Check for expected entries
	found := make(map[string]bool)
	for _, e := range entries {
		found[e.Name] = true
	}

	for _, expected := range []string{"app1", "app2", "sub"} {
		if !found[expected] {
			t.Errorf("expected entry %q not found in %v", expected, found)
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
	// Path format: secret/path/key - secret is at secret/vhist, key is "test"
	if err := client.Add(ctx, "secret/vhist/test", "v1"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := client.Update(ctx, "secret/vhist/test", "v2"); err != nil {
		t.Fatalf("Update to v2 failed: %v", err)
	}
	if err := client.Update(ctx, "secret/vhist/test", "v3"); err != nil {
		t.Fatalf("Update to v3 failed: %v", err)
	}

	// Get version history for the secret (not the key path)
	history, err := client.GetVersionHistory(ctx, "secret/vhist")
	if err != nil {
		t.Fatalf("GetVersionHistory failed: %v", err)
	}

	if len(history) < 3 {
		t.Errorf("expected at least 3 versions, got %d", len(history))
	}

	// Should be sorted descending (newest first)
	if len(history) > 0 && history[0].Version < 3 {
		t.Errorf("expected newest version >= 3, got %d", history[0].Version)
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
	// Path format: secret/path/key - secret is at secret/readver, key is "test"
	if err := client.Add(ctx, "secret/readver/test", "version-one"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := client.Update(ctx, "secret/readver/test", "version-two"); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Read version 1 - secret path is secret/readver, key is "test"
	v1Data, err := client.ReadSecretVersion(ctx, "secret/readver", 1)
	if err != nil {
		t.Fatalf("ReadSecretVersion failed: %v", err)
	}

	if v1Data["test"] != "version-one" {
		t.Errorf("expected 'version-one', got %v", v1Data["test"])
	}

	// Read version 2
	v2Data, err := client.ReadSecretVersion(ctx, "secret/readver", 2)
	if err != nil {
		t.Fatalf("ReadSecretVersion v2 failed: %v", err)
	}

	if v2Data["test"] != "version-two" {
		t.Errorf("expected 'version-two', got %v", v2Data["test"])
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

	// Create two secrets under snap/
	// secret/snap/app1 with key "key" and secret/snap/app2 with key "key"
	if err := client.Add(ctx, "secret/snap/app1/key", "value1"); err != nil {
		t.Fatalf("Add app1 failed: %v", err)
	}
	if err := client.Add(ctx, "secret/snap/app2/key", "value2"); err != nil {
		t.Fatalf("Add app2 failed: %v", err)
	}

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

	// Create secret and snapshot
	// Path: secret/restore/app/key creates secret at secret/restore/app with key "key"
	if err := client.Add(ctx, "secret/restore/app/key", "original"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	snapshot, err := client.CreateSnapshot(ctx, "secret/restore")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	// Modify the secret
	if err := client.Update(ctx, "secret/restore/app/key", "modified"); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Restore
	result, err := client.RestoreSnapshot(ctx, snapshot, "secret/restore", vault.RestoreOptions{})
	if err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}

	if len(result.Updated) != 1 {
		t.Errorf("expected 1 updated, got %d", len(result.Updated))
	}

	// Verify restored value - Get returns map with the key name
	secrets, err := client.Get(ctx, "secret/restore/app/key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if secrets["key"] != "original" {
		t.Errorf("expected 'original', got %v", secrets["key"])
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
	// All three keys are in the same secret at secret/dup
	if err := client.Add(ctx, "secret/dup/key1", "same-value"); err != nil {
		t.Fatalf("Add key1 failed: %v", err)
	}
	if err := client.Add(ctx, "secret/dup/key2", "same-value"); err != nil {
		t.Fatalf("Add key2 failed: %v", err)
	}
	if err := client.Add(ctx, "secret/dup/key3", "different"); err != nil {
		t.Fatalf("Add key3 failed: %v", err)
	}

	// Find duplicates
	duplicates, err := client.FindDuplicates(ctx, "secret/dup")
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}

	// Should have one set of duplicates (key1 and key2 have same value)
	if len(duplicates) != 1 {
		t.Errorf("expected 1 duplicate group, got %d", len(duplicates))
	}

	if len(duplicates) > 0 && len(duplicates[0].Paths) != 2 {
		t.Errorf("expected 2 paths in duplicate group, got %d", len(duplicates[0].Paths))
	}
}
