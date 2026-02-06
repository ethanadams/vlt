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

func TestIntegration_FindKeys(t *testing.T) {
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

	// Create secrets:
	// secret/find/app with keys: password, port, host
	// secret/find/sub/db with keys: password, connection_string
	if err := client.Add(ctx, "secret/find/app/password", "secret123"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := client.Add(ctx, "secret/find/app/port", "5432"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := client.Add(ctx, "secret/find/app/host", "localhost"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := client.Add(ctx, "secret/find/sub/db/password", "dbpass"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := client.Add(ctx, "secret/find/sub/db/connection_string", "postgres://..."); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	t.Run("non-recursive finds at immediate level", func(t *testing.T) {
		results, err := client.FindKeys(ctx, "secret/find", "p*", false)
		if err != nil {
			t.Fatalf("FindKeys failed: %v", err)
		}

		// Should find password and port in secret/find/app only (not nested sub/db)
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d: %+v", len(results), results)
		}
	})

	t.Run("recursive finds in nested secrets", func(t *testing.T) {
		results, err := client.FindKeys(ctx, "secret/find", "password", true)
		if err != nil {
			t.Fatalf("FindKeys failed: %v", err)
		}

		// Should find password in both app and sub/db
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d: %+v", len(results), results)
		}
	})

	t.Run("wildcard pattern", func(t *testing.T) {
		results, err := client.FindKeys(ctx, "secret/find", "*", true)
		if err != nil {
			t.Fatalf("FindKeys failed: %v", err)
		}

		// Should find all 5 keys
		if len(results) != 5 {
			t.Errorf("expected 5 results, got %d: %+v", len(results), results)
		}
	})

	t.Run("question mark glob", func(t *testing.T) {
		results, err := client.FindKeys(ctx, "secret/find", "por?", true)
		if err != nil {
			t.Fatalf("FindKeys failed: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d: %+v", len(results), results)
		}
		if len(results) > 0 && results[0].Key != "port" {
			t.Errorf("expected key 'port', got %q", results[0].Key)
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		results, err := client.FindKeys(ctx, "secret/find", "nonexistent*", true)
		if err != nil {
			t.Fatalf("FindKeys failed: %v", err)
		}

		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("find on single secret path", func(t *testing.T) {
		results, err := client.FindKeys(ctx, "secret/find/app", "h*", false)
		if err != nil {
			t.Fatalf("FindKeys failed: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d: %+v", len(results), results)
		}
		if len(results) > 0 && results[0].Key != "host" {
			t.Errorf("expected key 'host', got %q", results[0].Key)
		}
	})
}

func TestIntegration_KeyOperations(t *testing.T) {
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

	t.Run("WriteKey creates key in new secret", func(t *testing.T) {
		err := client.WriteKey(ctx, "secret/keyops", "mykey", "myvalue")
		if err != nil {
			t.Fatalf("WriteKey failed: %v", err)
		}

		val, err := client.ReadKey(ctx, "secret/keyops", "mykey")
		if err != nil {
			t.Fatalf("ReadKey failed: %v", err)
		}
		if val != "myvalue" {
			t.Errorf("expected 'myvalue', got %q", val)
		}
	})

	t.Run("WriteKey updates existing key", func(t *testing.T) {
		err := client.WriteKey(ctx, "secret/keyops", "mykey", "updated")
		if err != nil {
			t.Fatalf("WriteKey failed: %v", err)
		}

		val, err := client.ReadKey(ctx, "secret/keyops", "mykey")
		if err != nil {
			t.Fatalf("ReadKey failed: %v", err)
		}
		if val != "updated" {
			t.Errorf("expected 'updated', got %q", val)
		}
	})

	t.Run("WriteKey adds second key to same secret", func(t *testing.T) {
		err := client.WriteKey(ctx, "secret/keyops", "second", "val2")
		if err != nil {
			t.Fatalf("WriteKey failed: %v", err)
		}

		// Both keys should exist
		val1, err := client.ReadKey(ctx, "secret/keyops", "mykey")
		if err != nil {
			t.Fatalf("ReadKey mykey failed: %v", err)
		}
		if val1 != "updated" {
			t.Errorf("expected 'updated', got %q", val1)
		}

		val2, err := client.ReadKey(ctx, "secret/keyops", "second")
		if err != nil {
			t.Fatalf("ReadKey second failed: %v", err)
		}
		if val2 != "val2" {
			t.Errorf("expected 'val2', got %q", val2)
		}
	})

	t.Run("KeyExists returns true for existing key", func(t *testing.T) {
		exists, err := client.KeyExists(ctx, "secret/keyops", "mykey")
		if err != nil {
			t.Fatalf("KeyExists failed: %v", err)
		}
		if !exists {
			t.Error("expected key to exist")
		}
	})

	t.Run("KeyExists returns false for non-existing key", func(t *testing.T) {
		exists, err := client.KeyExists(ctx, "secret/keyops", "nonexistent")
		if err != nil {
			t.Fatalf("KeyExists failed: %v", err)
		}
		if exists {
			t.Error("expected key not to exist")
		}
	})

	t.Run("DeleteKey removes one key, leaves others", func(t *testing.T) {
		err := client.DeleteKey(ctx, "secret/keyops", "second")
		if err != nil {
			t.Fatalf("DeleteKey failed: %v", err)
		}

		// second should be gone
		exists, err := client.KeyExists(ctx, "secret/keyops", "second")
		if err != nil {
			t.Fatalf("KeyExists failed: %v", err)
		}
		if exists {
			t.Error("expected 'second' key to be deleted")
		}

		// mykey should still exist
		exists, err = client.KeyExists(ctx, "secret/keyops", "mykey")
		if err != nil {
			t.Fatalf("KeyExists failed: %v", err)
		}
		if !exists {
			t.Error("expected 'mykey' to still exist")
		}
	})

	t.Run("DeleteKey on last key deletes the secret", func(t *testing.T) {
		err := client.DeleteKey(ctx, "secret/keyops", "mykey")
		if err != nil {
			t.Fatalf("DeleteKey failed: %v", err)
		}

		exists, err := client.SecretExists(ctx, "secret/keyops")
		if err != nil {
			t.Fatalf("SecretExists failed: %v", err)
		}
		if exists {
			t.Error("expected secret to be deleted when last key removed")
		}
	})
}

func TestIntegration_ImportExport(t *testing.T) {
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

	t.Run("Import nested data, verify flattened keys", func(t *testing.T) {
		data := map[string]any{
			"admin": map[string]any{
				"password": "secret123",
				"username": "admin",
			},
			"db": map[string]any{
				"host": "localhost",
				"port": "5432",
			},
		}

		count, err := client.Import(ctx, "secret/impexp", data)
		if err != nil {
			t.Fatalf("Import failed: %v", err)
		}
		if count != 4 {
			t.Errorf("expected 4 keys imported, got %d", count)
		}

		// Verify flattened keys exist
		raw, err := client.ReadSecretRaw(ctx, "secret/impexp")
		if err != nil {
			t.Fatalf("ReadSecretRaw failed: %v", err)
		}
		if raw["admin.password"] != "secret123" {
			t.Errorf("expected admin.password='secret123', got %v", raw["admin.password"])
		}
		if raw["db.host"] != "localhost" {
			t.Errorf("expected db.host='localhost', got %v", raw["db.host"])
		}
	})

	t.Run("Export returns correct nested structure", func(t *testing.T) {
		exported, err := client.Export(ctx, "secret/impexp")
		if err != nil {
			t.Fatalf("Export failed: %v", err)
		}

		// Export uses ListSecrets which expands to nested structure
		if exported == nil {
			t.Fatal("expected non-nil export result")
		}

		// The exported structure should have the nested keys
		admin, ok := exported["admin"].(map[string]any)
		if !ok {
			t.Fatalf("expected admin to be a map, got %T", exported["admin"])
		}
		if admin["password"] != "secret123" {
			t.Errorf("expected admin.password='secret123', got %v", admin["password"])
		}
	})

	t.Run("Round-trip: import then export preserves data", func(t *testing.T) {
		data := map[string]any{
			"api": map[string]any{
				"key":      "abc123",
				"endpoint": "https://api.example.com",
			},
		}

		_, err := client.Import(ctx, "secret/roundtrip", data)
		if err != nil {
			t.Fatalf("Import failed: %v", err)
		}

		exported, err := client.Export(ctx, "secret/roundtrip")
		if err != nil {
			t.Fatalf("Export failed: %v", err)
		}

		api, ok := exported["api"].(map[string]any)
		if !ok {
			t.Fatalf("expected api to be a map, got %T", exported["api"])
		}
		if api["key"] != "abc123" {
			t.Errorf("expected api.key='abc123', got %v", api["key"])
		}
		if api["endpoint"] != "https://api.example.com" {
			t.Errorf("expected api.endpoint correct, got %v", api["endpoint"])
		}
	})
}

func TestIntegration_DeleteRecursive(t *testing.T) {
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

	// Create nested structure
	if err := client.Add(ctx, "secret/delrec/app/key", "v1"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := client.Add(ctx, "secret/delrec/sub/a/key", "v2"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := client.Add(ctx, "secret/delrec/sub/b/key", "v3"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	result, err := client.DeleteRecursive(ctx, "secret/delrec")
	if err != nil {
		t.Fatalf("DeleteRecursive failed: %v", err)
	}

	if result.Count != 3 {
		t.Errorf("expected 3 deleted, got %d", result.Count)
	}

	// Verify all are gone
	for _, path := range []string{"secret/delrec/app", "secret/delrec/sub/a", "secret/delrec/sub/b"} {
		exists, err := client.SecretExists(ctx, path)
		if err != nil {
			t.Fatalf("SecretExists(%s) failed: %v", path, err)
		}
		if exists {
			t.Errorf("expected %s to be deleted", path)
		}
	}
}

func TestIntegration_CopyRecursive(t *testing.T) {
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

	// Create source secrets
	if err := client.Add(ctx, "secret/cprecur/a/key", "val-a"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := client.Add(ctx, "secret/cprecur/b/key", "val-b"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	count, err := client.CopyRecursive(ctx, "secret/cprecur", "secret/cprecur-dst")
	if err != nil {
		t.Fatalf("CopyRecursive failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 copied, got %d", count)
	}

	// Verify destination has both secrets
	dstA, err := client.ReadSecretRaw(ctx, "secret/cprecur-dst/a")
	if err != nil {
		t.Fatalf("ReadSecretRaw dst/a failed: %v", err)
	}
	if dstA["key"] != "val-a" {
		t.Errorf("expected dst/a key='val-a', got %v", dstA["key"])
	}

	dstB, err := client.ReadSecretRaw(ctx, "secret/cprecur-dst/b")
	if err != nil {
		t.Fatalf("ReadSecretRaw dst/b failed: %v", err)
	}
	if dstB["key"] != "val-b" {
		t.Errorf("expected dst/b key='val-b', got %v", dstB["key"])
	}

	// Verify source still exists
	srcA, err := client.ReadSecretRaw(ctx, "secret/cprecur/a")
	if err != nil {
		t.Fatalf("ReadSecretRaw src/a failed: %v", err)
	}
	if srcA["key"] != "val-a" {
		t.Errorf("source should still exist after copy")
	}
}

func TestIntegration_MoveRecursive(t *testing.T) {
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

	// Create source secrets
	if err := client.Add(ctx, "secret/mvrecur/x/key", "val-x"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := client.Add(ctx, "secret/mvrecur/y/key", "val-y"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	count, err := client.MoveRecursive(ctx, "secret/mvrecur", "secret/mvrecur-dst")
	if err != nil {
		t.Fatalf("MoveRecursive failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 moved, got %d", count)
	}

	// Verify destination exists
	dstX, err := client.ReadSecretRaw(ctx, "secret/mvrecur-dst/x")
	if err != nil {
		t.Fatalf("ReadSecretRaw dst/x failed: %v", err)
	}
	if dstX["key"] != "val-x" {
		t.Errorf("expected dst/x key='val-x', got %v", dstX["key"])
	}

	// Verify source is gone
	exists, err := client.SecretExists(ctx, "secret/mvrecur/x")
	if err != nil {
		t.Fatalf("SecretExists failed: %v", err)
	}
	if exists {
		t.Error("source should not exist after move")
	}

	exists, err = client.SecretExists(ctx, "secret/mvrecur/y")
	if err != nil {
		t.Fatalf("SecretExists failed: %v", err)
	}
	if exists {
		t.Error("source y should not exist after move")
	}
}

func TestIntegration_CompareVersions(t *testing.T) {
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

	// Create secret at secret/cmpver with keys: keep, change, remove
	if err := client.WriteSecret(ctx, "secret/cmpver", map[string]any{
		"keep":   "same",
		"change": "old",
		"remove": "gone",
	}); err != nil {
		t.Fatalf("WriteSecret v1 failed: %v", err)
	}

	// Update: modify "change", delete "remove", add "added"
	if err := client.WriteSecret(ctx, "secret/cmpver", map[string]any{
		"keep":   "same",
		"change": "new",
		"added":  "fresh",
	}); err != nil {
		t.Fatalf("WriteSecret v2 failed: %v", err)
	}

	changes, err := client.CompareVersions(ctx, "secret/cmpver", 1, 2)
	if err != nil {
		t.Fatalf("CompareVersions failed: %v", err)
	}

	// Should have: 1 added (added), 1 modified (change), 1 deleted (remove)
	var added, modified, deleted int
	for _, c := range changes {
		switch c.Type {
		case vault.ChangeAdded:
			added++
			if c.Key != "added" {
				t.Errorf("unexpected added key: %q", c.Key)
			}
		case vault.ChangeModified:
			modified++
			if c.Key != "change" {
				t.Errorf("unexpected modified key: %q", c.Key)
			}
			if c.OldValue != "old" || c.NewValue != "new" {
				t.Errorf("unexpected values: old=%q new=%q", c.OldValue, c.NewValue)
			}
		case vault.ChangeDeleted:
			deleted++
			if c.Key != "remove" {
				t.Errorf("unexpected deleted key: %q", c.Key)
			}
		}
	}

	if added != 1 {
		t.Errorf("expected 1 added, got %d", added)
	}
	if modified != 1 {
		t.Errorf("expected 1 modified, got %d", modified)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}
}

func TestIntegration_Timeline(t *testing.T) {
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

	// Create secrets with multiple versions under secret/timeline
	if err := client.Add(ctx, "secret/timeline/app/config", "v1"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := client.Add(ctx, "secret/timeline/db/password", "v1"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	// Update to create v2 for each
	if err := client.Update(ctx, "secret/timeline/app/config", "v2"); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if err := client.Update(ctx, "secret/timeline/db/password", "v2"); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	t.Run("GetTimeline returns chronological changes", func(t *testing.T) {
		timeline, err := client.GetTimeline(ctx, "secret/timeline")
		if err != nil {
			t.Fatalf("GetTimeline failed: %v", err)
		}

		// Should have 4 entries total (2 secrets x 2 versions each)
		if len(timeline) != 4 {
			t.Errorf("expected 4 timeline entries, got %d", len(timeline))
		}

		// Verify sorted newest first
		for i := 1; i < len(timeline); i++ {
			if timeline[i].Time.After(timeline[i-1].Time) {
				t.Errorf("timeline not sorted descending at index %d", i)
			}
		}
	})

	t.Run("GetPrevVersions returns previous version of each secret", func(t *testing.T) {
		prevVersions, err := client.GetPrevVersions(ctx, "secret/timeline")
		if err != nil {
			t.Fatalf("GetPrevVersions failed: %v", err)
		}

		// Should have 2 entries (one per secret, each showing previous version)
		if len(prevVersions) != 2 {
			t.Errorf("expected 2 prev version entries, got %d", len(prevVersions))
		}

		// Both should have value "v1" (previous version)
		for key, val := range prevVersions {
			if val != "v1" {
				t.Errorf("expected prev version value 'v1' for %s, got %v", key, val)
			}
		}
	})

	t.Run("GetStateAtChangesAgo computes correct historical state", func(t *testing.T) {
		// 1 change ago should undo the most recent change
		state, err := client.GetStateAtChangesAgo(ctx, "secret/timeline", 1)
		if err != nil {
			t.Fatalf("GetStateAtChangesAgo failed: %v", err)
		}

		if state == nil {
			t.Fatal("expected non-nil state")
		}

		// At least one secret should still be at v1
		hasV1 := false
		for _, val := range state {
			if val == "v1" {
				hasV1 = true
			}
		}
		if !hasV1 {
			t.Error("expected at least one secret to be at v1 state")
		}
	})

	t.Run("GetSecretAtVersion reads specific version", func(t *testing.T) {
		secrets, err := client.GetSecretAtVersion(ctx, "secret/timeline/app", 1, false)
		if err != nil {
			t.Fatalf("GetSecretAtVersion failed: %v", err)
		}

		if secrets["config"] != "v1" {
			t.Errorf("expected config='v1', got %v", secrets["config"])
		}
	})

	t.Run("GetSecretAtVersion with isPrev", func(t *testing.T) {
		secrets, err := client.GetSecretAtVersion(ctx, "secret/timeline/app", 0, true)
		if err != nil {
			t.Fatalf("GetSecretAtVersion isPrev failed: %v", err)
		}

		if secrets["config"] != "v1" {
			t.Errorf("expected config='v1' for previous, got %v", secrets["config"])
		}
	})
}

func TestIntegration_Tree(t *testing.T) {
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

	// Create tree structure:
	// secret/tree/
	//   config  (secret with key "val")
	//   sub/
	//     app   (secret with key "val")
	//     db    (secret with key "val")
	if err := client.Add(ctx, "secret/tree/config/val", "cfg"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := client.Add(ctx, "secret/tree/sub/app/val", "app-val"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := client.Add(ctx, "secret/tree/sub/db/val", "db-val"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	t.Run("GetTree builds correct structure", func(t *testing.T) {
		tree, err := client.GetTree(ctx, "secret/tree")
		if err != nil {
			t.Fatalf("GetTree failed: %v", err)
		}

		if tree == nil {
			t.Fatal("expected non-nil tree")
		}

		// Root should be a directory
		if !tree.IsDir {
			t.Error("root should be a directory")
		}

		// Count secrets and dirs
		if tree.CountSecrets() != 3 {
			t.Errorf("expected 3 secrets, got %d", tree.CountSecrets())
		}
		if tree.CountDirs() != 1 {
			t.Errorf("expected 1 directory (sub), got %d", tree.CountDirs())
		}
	})

	t.Run("GetTreeWithMetadata includes version info", func(t *testing.T) {
		tree, err := client.GetTreeWithMetadata(ctx, "secret/tree")
		if err != nil {
			t.Fatalf("GetTreeWithMetadata failed: %v", err)
		}

		// Walk the tree and check that leaf nodes have metadata
		hasMetadata := false
		tree.Walk(func(node *vault.TreeNode, depth int, isLast bool) {
			if !node.IsDir && node.Metadata != nil {
				hasMetadata = true
				if node.Metadata.CurrentVersion < 1 {
					t.Errorf("expected version >= 1, got %d for %s", node.Metadata.CurrentVersion, node.Name)
				}
			}
		})

		if !hasMetadata {
			t.Error("expected at least one leaf with metadata")
		}
	})
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
