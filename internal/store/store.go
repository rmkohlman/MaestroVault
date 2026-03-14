// Package store provides SQLite-backed storage for encrypted secrets.
// Only encrypted data is ever written to disk — plaintext values never
// touch the database.
//
// The SecretStore interface is the contract for all storage operations.
// SQLiteStore is the concrete implementation backed by modernc.org/sqlite.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // SQLite driver
)

// ── Sentinel errors ──────────────────────────────────────────

var (
	// ErrNotFound is returned when a requested secret does not exist.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists is returned when a secret with the same
	// (name, environment) pair already exists and an insert-only
	// operation was attempted. Note: Put uses UPSERT, so this error
	// is only returned when explicitly checking for duplicates.
	ErrAlreadyExists = errors.New("already exists")
)

// ── Models ───────────────────────────────────────────────────

// Secret represents a stored secret with its encrypted data and metadata.
// This is the storage-layer model — values are always encrypted.
type Secret struct {
	ID               int64
	Name             string
	Environment      string
	EncryptedSecret  []byte
	EncryptedDataKey []byte
	Metadata         json.RawMessage
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// NameEntry pairs a secret name with its environment for shell completions
// and listing operations.
type NameEntry struct {
	Name        string
	Environment string
}

// ── Interface ────────────────────────────────────────────────

// SecretStore defines the contract for encrypted secret persistence.
// All operations are context-aware and environment-scoped.
//
// The unique key for a secret is the (name, environment) pair.
// Passing env="" to List means "all environments".
type SecretStore interface {
	// Put stores or updates an encrypted secret (UPSERT on name+environment).
	Put(ctx context.Context, name, env string, encSecret, encDataKey []byte, metadata json.RawMessage) error

	// Get retrieves an encrypted secret by name and environment.
	// Returns ErrNotFound if the secret does not exist.
	Get(ctx context.Context, name, env string) (*Secret, error)

	// List returns all stored secrets ordered by name.
	// If env is non-empty, only secrets in that environment are returned.
	// If env is empty, secrets from all environments are returned.
	List(ctx context.Context, env string) ([]Secret, error)

	// Delete removes a secret by name and environment.
	// Returns ErrNotFound if the secret does not exist.
	Delete(ctx context.Context, name, env string) error

	// Search returns secrets whose name, environment, or metadata match the query.
	Search(ctx context.Context, query string) ([]Secret, error)

	// ListByMetadata returns secrets where a metadata key matches the given value.
	// Uses json_extract() for reliable JSON querying.
	// If value is empty, matches any secret that has the given key.
	ListByMetadata(ctx context.Context, key, value string) ([]Secret, error)

	// Count returns the total number of secrets in the store.
	Count(ctx context.Context) (int, error)

	// Names returns all (name, environment) pairs, sorted by name.
	// Useful for shell completions.
	Names(ctx context.Context) ([]NameEntry, error)

	// Close releases all database resources.
	Close() error

	// DB returns the underlying database handle for sharing with
	// components that need direct access (e.g., API token store).
	DB() *sql.DB
}

// ── Concrete implementation ──────────────────────────────────

// SQLiteStore implements SecretStore backed by a SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at dbPath, runs migrations,
// and returns an *SQLiteStore that satisfies the SecretStore interface.
func New(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode and foreign keys.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set journal_mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set foreign_keys: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Compile-time assertion: *SQLiteStore implements SecretStore.
var _ SecretStore = (*SQLiteStore)(nil)

// ── Migration ────────────────────────────────────────────────

const schemaVersion = 2

// v2 fresh schema SQL.
const v2SchemaSQL = `
CREATE TABLE IF NOT EXISTS secrets (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    name               TEXT NOT NULL,
    environment        TEXT NOT NULL DEFAULT '',
    encrypted_secret   BLOB NOT NULL,
    encrypted_data_key BLOB NOT NULL,
    metadata           TEXT NOT NULL DEFAULT '{}',
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, environment)
);
CREATE INDEX IF NOT EXISTS idx_secrets_name ON secrets(name);
CREATE INDEX IF NOT EXISTS idx_secrets_env ON secrets(environment);

CREATE TABLE IF NOT EXISTS api_tokens (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    token_hash  TEXT UNIQUE NOT NULL,
    salt        TEXT NOT NULL DEFAULT '',
    scopes      TEXT NOT NULL DEFAULT '[]',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at  DATETIME,
    last_used_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash);
`

// v0 → v2 migration SQL.
// SQLite does not support dropping column-level UNIQUE constraints, so
// we rebuild the secrets table using the rename-create-copy-drop pattern.
const v0ToV2MigrationSQL = `
-- Rebuild secrets table with new schema
ALTER TABLE secrets RENAME TO _secrets_old;
DROP INDEX IF EXISTS idx_secrets_name;

CREATE TABLE secrets (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    name               TEXT NOT NULL,
    environment        TEXT NOT NULL DEFAULT '',
    encrypted_secret   BLOB NOT NULL,
    encrypted_data_key BLOB NOT NULL,
    metadata           TEXT NOT NULL DEFAULT '{}',
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, environment)
);

INSERT INTO secrets (id, name, environment, encrypted_secret, encrypted_data_key, metadata, created_at, updated_at)
    SELECT id, name, '', encrypted_value, encrypted_data_key, labels, created_at, updated_at
    FROM _secrets_old;

DROP TABLE _secrets_old;

CREATE INDEX idx_secrets_name ON secrets(name);
CREATE INDEX idx_secrets_env ON secrets(environment);

-- Add salt column to api_tokens
ALTER TABLE api_tokens ADD COLUMN salt TEXT NOT NULL DEFAULT '';
`

func (s *SQLiteStore) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}

	switch version {
	case schemaVersion:
		// Already at v2, nothing to do.
		return nil
	case 0:
		// Check if tables already exist (v0 legacy) or this is a fresh install.
		var tableName string
		err := s.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name='secrets'",
		).Scan(&tableName)

		if errors.Is(err, sql.ErrNoRows) {
			// Fresh install — create v2 schema directly.
			if _, err := s.db.Exec(v2SchemaSQL); err != nil {
				return fmt.Errorf("create v2 schema: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("check existing tables: %w", err)
		} else {
			// Legacy v0 tables exist — run ALTER TABLE migration.
			if _, err := s.db.Exec(v0ToV2MigrationSQL); err != nil {
				return fmt.Errorf("migrate v0 to v2: %w", err)
			}
		}

		// Set the schema version.
		if _, err := s.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
			return fmt.Errorf("set user_version: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported schema version %d (expected %d)", version, schemaVersion)
	}
}

// ── SecretStore method implementations ───────────────────────

func (s *SQLiteStore) Put(ctx context.Context, name, env string, encSecret, encDataKey []byte, metadata json.RawMessage) error {
	if metadata == nil {
		metadata = json.RawMessage("{}")
	}

	const query = `
		INSERT INTO secrets (name, environment, encrypted_secret, encrypted_data_key, metadata, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(name, environment) DO UPDATE SET
		    encrypted_secret = excluded.encrypted_secret,
		    encrypted_data_key = excluded.encrypted_data_key,
		    metadata = excluded.metadata,
		    updated_at = CURRENT_TIMESTAMP`

	_, err := s.db.ExecContext(ctx, query, name, env, encSecret, encDataKey, string(metadata))
	if err != nil {
		return fmt.Errorf("put secret %q (env=%q): %w", name, env, err)
	}
	return nil
}

func (s *SQLiteStore) Get(ctx context.Context, name, env string) (*Secret, error) {
	const query = `
		SELECT id, name, environment, encrypted_secret, encrypted_data_key, metadata, created_at, updated_at
		FROM secrets
		WHERE name = ? AND environment = ?`

	var sec Secret
	var meta string
	err := s.db.QueryRowContext(ctx, query, name, env).Scan(
		&sec.ID, &sec.Name, &sec.Environment,
		&sec.EncryptedSecret, &sec.EncryptedDataKey,
		&meta, &sec.CreatedAt, &sec.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get secret %q (env=%q): %w", name, env, err)
	}
	sec.Metadata = json.RawMessage(meta)
	return &sec, nil
}

func (s *SQLiteStore) List(ctx context.Context, env string) ([]Secret, error) {
	var rows *sql.Rows
	var err error

	if env == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, name, environment, encrypted_secret, encrypted_data_key, metadata, created_at, updated_at
			 FROM secrets ORDER BY name`)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, name, environment, encrypted_secret, encrypted_data_key, metadata, created_at, updated_at
			 FROM secrets WHERE environment = ? ORDER BY name`, env)
	}
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	defer rows.Close()
	return scanSecrets(rows)
}

func (s *SQLiteStore) Delete(ctx context.Context, name, env string) error {
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM secrets WHERE name = ? AND environment = ?", name, env)
	if err != nil {
		return fmt.Errorf("delete secret %q (env=%q): %w", name, env, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) Search(ctx context.Context, query string) ([]Secret, error) {
	pattern := "%" + query + "%"
	const q = `
		SELECT id, name, environment, encrypted_secret, encrypted_data_key, metadata, created_at, updated_at
		FROM secrets
		WHERE name LIKE ? COLLATE NOCASE
		   OR environment LIKE ? COLLATE NOCASE
		   OR metadata LIKE ? COLLATE NOCASE
		ORDER BY name`

	rows, err := s.db.QueryContext(ctx, q, pattern, pattern, pattern)
	if err != nil {
		return nil, fmt.Errorf("search secrets: %w", err)
	}
	defer rows.Close()
	return scanSecrets(rows)
}

func (s *SQLiteStore) ListByMetadata(ctx context.Context, key, value string) ([]Secret, error) {
	var rows *sql.Rows
	var err error

	// Note: The key is passed as a parameterized value via '$.' || ?, so
	// SQLite treats it as a literal string — there is no SQL injection risk.
	// However, keys containing JSON path special characters (e.g. dots,
	// brackets) may produce unexpected json_extract results. This is an
	// accepted edge case for the metadata key naming convention.
	if value == "" {
		// Match any secret that has the key (json_extract returns non-null).
		const q = `
			SELECT id, name, environment, encrypted_secret, encrypted_data_key, metadata, created_at, updated_at
			FROM secrets
			WHERE json_extract(metadata, '$.' || ?) IS NOT NULL
			ORDER BY name`
		rows, err = s.db.QueryContext(ctx, q, key)
	} else {
		// Match exact value for the given key.
		const q = `
			SELECT id, name, environment, encrypted_secret, encrypted_data_key, metadata, created_at, updated_at
			FROM secrets
			WHERE json_extract(metadata, '$.' || ?) = ?
			ORDER BY name`
		rows, err = s.db.QueryContext(ctx, q, key, value)
	}
	if err != nil {
		return nil, fmt.Errorf("list by metadata: %w", err)
	}
	defer rows.Close()
	return scanSecrets(rows)
}

func (s *SQLiteStore) Count(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM secrets").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count secrets: %w", err)
	}
	return count, nil
}

func (s *SQLiteStore) Names(ctx context.Context) ([]NameEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT name, environment FROM secrets ORDER BY name, environment")
	if err != nil {
		return nil, fmt.Errorf("list names: %w", err)
	}
	defer rows.Close()

	var entries []NameEntry
	for rows.Next() {
		var e NameEntry
		if err := rows.Scan(&e.Name, &e.Environment); err != nil {
			return nil, fmt.Errorf("scan name entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

// ── Helpers ──────────────────────────────────────────────────

// scanSecrets scans all rows into a slice of Secret.
func scanSecrets(rows *sql.Rows) ([]Secret, error) {
	var secrets []Secret
	for rows.Next() {
		var sec Secret
		var meta string
		if err := rows.Scan(
			&sec.ID, &sec.Name, &sec.Environment,
			&sec.EncryptedSecret, &sec.EncryptedDataKey,
			&meta, &sec.CreatedAt, &sec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan secret: %w", err)
		}
		sec.Metadata = json.RawMessage(meta)
		secrets = append(secrets, sec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate secrets: %w", err)
	}
	return secrets, nil
}
