package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPutAndGet(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	meta := json.RawMessage(`{"team":"backend","tier":"critical"}`)
	if err := s.Put(ctx, "db-password", "production", []byte("enc-value"), []byte("enc-key"), meta); err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	secret, err := s.Get(ctx, "db-password", "production")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}

	if secret.Name != "db-password" {
		t.Errorf("name: want %q, got %q", "db-password", secret.Name)
	}
	if secret.Environment != "production" {
		t.Errorf("environment: want %q, got %q", "production", secret.Environment)
	}
	if string(secret.EncryptedSecret) != "enc-value" {
		t.Errorf("encrypted_secret: want %q, got %q", "enc-value", secret.EncryptedSecret)
	}
	if string(secret.EncryptedDataKey) != "enc-key" {
		t.Errorf("encrypted_data_key: want %q, got %q", "enc-key", secret.EncryptedDataKey)
	}
	if string(secret.Metadata) != string(meta) {
		t.Errorf("metadata: want %s, got %s", meta, secret.Metadata)
	}
	if secret.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if secret.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
	if secret.UpdatedAt.IsZero() {
		t.Error("expected non-zero UpdatedAt")
	}
}

func TestPutUpsert(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_ = s.Put(ctx, "api-key", "staging", []byte("v1"), []byte("k1"), nil)
	_ = s.Put(ctx, "api-key", "staging", []byte("v2"), []byte("k2"), nil)

	secret, err := s.Get(ctx, "api-key", "staging")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(secret.EncryptedSecret) != "v2" {
		t.Errorf("expected updated value %q, got %q", "v2", secret.EncryptedSecret)
	}
	if string(secret.EncryptedDataKey) != "k2" {
		t.Errorf("expected updated key %q, got %q", "k2", secret.EncryptedDataKey)
	}

	// Should still be one row, not two.
	count, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 secret after upsert, got %d", count)
	}
}

func TestDifferentEnvironmentsSameName(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_ = s.Put(ctx, "db-password", "production", []byte("prod-val"), []byte("prod-key"), nil)
	_ = s.Put(ctx, "db-password", "staging", []byte("stage-val"), []byte("stage-key"), nil)

	prod, err := s.Get(ctx, "db-password", "production")
	if err != nil {
		t.Fatalf("Get(production) error: %v", err)
	}
	if string(prod.EncryptedSecret) != "prod-val" {
		t.Errorf("prod value: want %q, got %q", "prod-val", prod.EncryptedSecret)
	}

	stage, err := s.Get(ctx, "db-password", "staging")
	if err != nil {
		t.Fatalf("Get(staging) error: %v", err)
	}
	if string(stage.EncryptedSecret) != "stage-val" {
		t.Errorf("stage value: want %q, got %q", "stage-val", stage.EncryptedSecret)
	}

	count, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 secrets, got %d", count)
	}
}

func TestGetNotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_, err := s.Get(ctx, "nonexistent", "")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestListAll(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_ = s.Put(ctx, "charlie", "prod", []byte("v3"), []byte("k3"), nil)
	_ = s.Put(ctx, "alpha", "staging", []byte("v1"), []byte("k1"), nil)
	_ = s.Put(ctx, "bravo", "prod", []byte("v2"), []byte("k2"), nil)

	secrets, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(secrets) != 3 {
		t.Fatalf("expected 3 secrets, got %d", len(secrets))
	}
	// Should be sorted by name.
	if secrets[0].Name != "alpha" {
		t.Errorf("first: want %q, got %q", "alpha", secrets[0].Name)
	}
	if secrets[1].Name != "bravo" {
		t.Errorf("second: want %q, got %q", "bravo", secrets[1].Name)
	}
	if secrets[2].Name != "charlie" {
		t.Errorf("third: want %q, got %q", "charlie", secrets[2].Name)
	}
}

func TestListByEnvironment(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_ = s.Put(ctx, "alpha", "prod", []byte("v1"), []byte("k1"), nil)
	_ = s.Put(ctx, "bravo", "staging", []byte("v2"), []byte("k2"), nil)
	_ = s.Put(ctx, "charlie", "prod", []byte("v3"), []byte("k3"), nil)

	prodSecrets, err := s.List(ctx, "prod")
	if err != nil {
		t.Fatalf("List(prod) error: %v", err)
	}
	if len(prodSecrets) != 2 {
		t.Fatalf("expected 2 prod secrets, got %d", len(prodSecrets))
	}
	if prodSecrets[0].Name != "alpha" || prodSecrets[1].Name != "charlie" {
		t.Errorf("unexpected prod secrets: %v, %v", prodSecrets[0].Name, prodSecrets[1].Name)
	}

	stagingSecrets, err := s.List(ctx, "staging")
	if err != nil {
		t.Fatalf("List(staging) error: %v", err)
	}
	if len(stagingSecrets) != 1 {
		t.Fatalf("expected 1 staging secret, got %d", len(stagingSecrets))
	}
	if stagingSecrets[0].Name != "bravo" {
		t.Errorf("expected bravo, got %q", stagingSecrets[0].Name)
	}
}

func TestListEmpty(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	secrets, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("expected 0 secrets, got %d", len(secrets))
	}
}

func TestDelete(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_ = s.Put(ctx, "temp", "dev", []byte("v"), []byte("k"), nil)

	if err := s.Delete(ctx, "temp", "dev"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err := s.Get(ctx, "temp", "dev")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	err := s.Delete(ctx, "nonexistent", "")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestDeleteScopedToEnvironment(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_ = s.Put(ctx, "secret", "prod", []byte("v1"), []byte("k1"), nil)
	_ = s.Put(ctx, "secret", "staging", []byte("v2"), []byte("k2"), nil)

	// Deleting from prod should not affect staging.
	if err := s.Delete(ctx, "secret", "prod"); err != nil {
		t.Fatalf("Delete(prod) error: %v", err)
	}

	_, err := s.Get(ctx, "secret", "prod")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected prod to be deleted, got: %v", err)
	}

	stage, err := s.Get(ctx, "secret", "staging")
	if err != nil {
		t.Fatalf("staging should still exist: %v", err)
	}
	if string(stage.EncryptedSecret) != "v2" {
		t.Errorf("staging value: want %q, got %q", "v2", stage.EncryptedSecret)
	}
}

func TestSearch(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_ = s.Put(ctx, "db-password", "production", []byte("v1"), []byte("k1"),
		json.RawMessage(`{"team":"backend"}`))
	_ = s.Put(ctx, "api-key", "staging", []byte("v2"), []byte("k2"),
		json.RawMessage(`{"team":"frontend"}`))
	_ = s.Put(ctx, "ssh-key", "production", []byte("v3"), []byte("k3"),
		json.RawMessage(`{"team":"devops"}`))

	// Search by name.
	results, err := s.Search(ctx, "password")
	if err != nil {
		t.Fatalf("Search(password) error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "db-password" {
		t.Errorf("search by name: expected [db-password], got %v", secretNames(results))
	}

	// Search by environment.
	results, err = s.Search(ctx, "staging")
	if err != nil {
		t.Fatalf("Search(staging) error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "api-key" {
		t.Errorf("search by env: expected [api-key], got %v", secretNames(results))
	}

	// Search by metadata content.
	results, err = s.Search(ctx, "devops")
	if err != nil {
		t.Fatalf("Search(devops) error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "ssh-key" {
		t.Errorf("search by metadata: expected [ssh-key], got %v", secretNames(results))
	}

	// Search matching multiple (case insensitive).
	results, err = s.Search(ctx, "KEY")
	if err != nil {
		t.Fatalf("Search(KEY) error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("search KEY: expected 2 results, got %d", len(results))
	}
}

func TestListByMetadata(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_ = s.Put(ctx, "secret-a", "", []byte("v1"), []byte("k1"),
		json.RawMessage(`{"team":"backend","tier":"critical"}`))
	_ = s.Put(ctx, "secret-b", "", []byte("v2"), []byte("k2"),
		json.RawMessage(`{"team":"frontend","tier":"low"}`))
	_ = s.Put(ctx, "secret-c", "", []byte("v3"), []byte("k3"),
		json.RawMessage(`{"team":"backend"}`))

	// Exact match on key+value.
	results, err := s.ListByMetadata(ctx, "team", "backend")
	if err != nil {
		t.Fatalf("ListByMetadata(team, backend) error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for team=backend, got %d", len(results))
	}

	// Exact match on different value.
	results, err = s.ListByMetadata(ctx, "tier", "critical")
	if err != nil {
		t.Fatalf("ListByMetadata(tier, critical) error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "secret-a" {
		t.Errorf("expected [secret-a], got %v", secretNames(results))
	}

	// Key exists (value empty = match any with key).
	results, err = s.ListByMetadata(ctx, "tier", "")
	if err != nil {
		t.Fatalf("ListByMetadata(tier, '') error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results with 'tier' key, got %d", len(results))
	}

	// Key does not exist in any secret.
	results, err = s.ListByMetadata(ctx, "nonexistent", "")
	if err != nil {
		t.Fatalf("ListByMetadata(nonexistent) error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for nonexistent key, got %d", len(results))
	}
}

func TestCount(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	count, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	_ = s.Put(ctx, "a", "", []byte("v"), []byte("k"), nil)
	_ = s.Put(ctx, "b", "prod", []byte("v"), []byte("k"), nil)
	_ = s.Put(ctx, "c", "staging", []byte("v"), []byte("k"), nil)

	count, err = s.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestNames(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_ = s.Put(ctx, "charlie", "prod", []byte("v"), []byte("k"), nil)
	_ = s.Put(ctx, "alpha", "staging", []byte("v"), []byte("k"), nil)
	_ = s.Put(ctx, "alpha", "prod", []byte("v"), []byte("k"), nil)
	_ = s.Put(ctx, "bravo", "", []byte("v"), []byte("k"), nil)

	names, err := s.Names(ctx)
	if err != nil {
		t.Fatalf("Names() error: %v", err)
	}
	if len(names) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(names))
	}

	// Should be ordered by name, then environment.
	expected := []NameEntry{
		{Name: "alpha", Environment: "prod"},
		{Name: "alpha", Environment: "staging"},
		{Name: "bravo", Environment: ""},
		{Name: "charlie", Environment: "prod"},
	}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("names[%d]: want %+v, got %+v", i, want, names[i])
		}
	}
}

func TestNilMetadataDefaultsToEmptyObject(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.Put(ctx, "no-meta", "", []byte("v"), []byte("k"), nil); err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	secret, err := s.Get(ctx, "no-meta", "")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(secret.Metadata) != "{}" {
		t.Errorf("metadata: want %q, got %q", "{}", string(secret.Metadata))
	}
}

func TestFreshInstallCreatesV2Schema(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fresh.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer s.Close()

	// Verify user_version is 2.
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	if version != 2 {
		t.Errorf("user_version: want 2, got %d", version)
	}

	// Verify secrets table has expected columns.
	ctx := context.Background()
	if err := s.Put(ctx, "test", "env", []byte("v"), []byte("k"), json.RawMessage(`{"a":"b"}`)); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	sec, err := s.Get(ctx, "test", "env")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if sec.Environment != "env" {
		t.Errorf("environment: want %q, got %q", "env", sec.Environment)
	}

	// Verify api_tokens table exists with salt column.
	_, err = s.db.Exec(
		"INSERT INTO api_tokens (id, name, token_hash, salt, scopes) VALUES ('t1', 'test', 'hash', 'salt', '[]')")
	if err != nil {
		t.Fatalf("api_tokens insert error: %v", err)
	}
}

func TestMigrationFromV0Schema(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	// Create a v0 schema database manually.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	v0Schema := `
		CREATE TABLE secrets (
			id                 INTEGER PRIMARY KEY AUTOINCREMENT,
			name               TEXT UNIQUE NOT NULL,
			encrypted_value    BLOB NOT NULL,
			encrypted_data_key BLOB NOT NULL,
			labels             TEXT NOT NULL DEFAULT '{}',
			created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE UNIQUE INDEX idx_secrets_name ON secrets(name);

		CREATE TABLE api_tokens (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			token_hash  TEXT UNIQUE NOT NULL,
			scopes      TEXT NOT NULL DEFAULT '[]',
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at  DATETIME,
			last_used_at DATETIME
		);
		CREATE INDEX idx_api_tokens_hash ON api_tokens(token_hash);
	`
	if _, err := db.Exec(v0Schema); err != nil {
		t.Fatalf("create v0 schema: %v", err)
	}

	// Insert some v0 data.
	_, err = db.Exec(
		`INSERT INTO secrets (name, encrypted_value, encrypted_data_key, labels) VALUES (?, ?, ?, ?)`,
		"old-secret", []byte("old-val"), []byte("old-key"), `{"env":"dev"}`)
	if err != nil {
		t.Fatalf("insert v0 data: %v", err)
	}
	_, err = db.Exec(
		`INSERT INTO api_tokens (id, name, token_hash, scopes) VALUES (?, ?, ?, ?)`,
		"tok1", "test-token", "hash123", "[]")
	if err != nil {
		t.Fatalf("insert v0 token: %v", err)
	}
	db.Close()

	// Now open with New(), which should trigger migration.
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() on v0 db error: %v", err)
	}
	defer s.Close()

	// Verify user_version is 2.
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	if version != 2 {
		t.Errorf("user_version after migration: want 2, got %d", version)
	}

	// Verify old data is accessible with new column names.
	ctx := context.Background()
	secret, err := s.Get(ctx, "old-secret", "") // environment defaults to ''
	if err != nil {
		t.Fatalf("Get() after migration error: %v", err)
	}
	if string(secret.EncryptedSecret) != "old-val" {
		t.Errorf("encrypted_secret: want %q, got %q", "old-val", secret.EncryptedSecret)
	}
	if string(secret.EncryptedDataKey) != "old-key" {
		t.Errorf("encrypted_data_key: want %q, got %q", "old-key", secret.EncryptedDataKey)
	}
	if string(secret.Metadata) != `{"env":"dev"}` {
		t.Errorf("metadata: want %q, got %q", `{"env":"dev"}`, string(secret.Metadata))
	}
	if secret.Environment != "" {
		t.Errorf("environment: want empty, got %q", secret.Environment)
	}

	// Verify we can insert with environment now.
	if err := s.Put(ctx, "old-secret", "prod", []byte("new-val"), []byte("new-key"), nil); err != nil {
		t.Fatalf("Put() after migration error: %v", err)
	}

	// Both should coexist.
	count, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 secrets after migration + insert, got %d", count)
	}

	// Verify api_tokens salt column exists.
	var salt string
	err = s.db.QueryRow("SELECT salt FROM api_tokens WHERE id = 'tok1'").Scan(&salt)
	if err != nil {
		t.Fatalf("query salt: %v", err)
	}
	if salt != "" {
		t.Errorf("expected empty default salt, got %q", salt)
	}
}

func TestRepeatedOpenDoesNotReMigrate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "reopen.db")

	s1, err := New(dbPath)
	if err != nil {
		t.Fatalf("first New() error: %v", err)
	}
	ctx := context.Background()
	_ = s1.Put(ctx, "persist", "env", []byte("v"), []byte("k"), nil)
	s1.Close()

	// Re-open the same database.
	s2, err := New(dbPath)
	if err != nil {
		t.Fatalf("second New() error: %v", err)
	}
	defer s2.Close()

	secret, err := s2.Get(ctx, "persist", "env")
	if err != nil {
		t.Fatalf("Get() after reopen: %v", err)
	}
	if string(secret.EncryptedSecret) != "v" {
		t.Errorf("value: want %q, got %q", "v", secret.EncryptedSecret)
	}
}

func TestDatabaseFileCreated(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "vault.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	s.Close()
}

func TestDBReturnsHandle(t *testing.T) {
	s := testStore(t)
	if s.DB() == nil {
		t.Fatal("DB() returned nil")
	}
}

// secretNames is a test helper for readable error messages.
func secretNames(secrets []Secret) []string {
	names := make([]string, len(secrets))
	for i, s := range secrets {
		names[i] = fmt.Sprintf("%s(%s)", s.Name, s.Environment)
	}
	return names
}
