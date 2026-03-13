// Package store provides SQLite-backed storage for encrypted secrets.
// Only encrypted data is ever written to disk — plaintext values never
// touch the database.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Secret represents a stored secret with its encrypted data and metadata.
type Secret struct {
	ID               int64
	Name             string
	EncryptedValue   []byte
	EncryptedDataKey []byte
	Labels           map[string]string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Store manages the SQLite database for encrypted secrets.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at dbPath and runs migrations.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrent read access.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return s, nil
}

func (s *Store) migrate() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS secrets (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		name             TEXT UNIQUE NOT NULL,
		encrypted_value  BLOB NOT NULL,
		encrypted_data_key BLOB NOT NULL,
		labels           TEXT NOT NULL DEFAULT '{}',
		created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_secrets_name ON secrets(name);

	CREATE TABLE IF NOT EXISTS api_tokens (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		token_hash  TEXT UNIQUE NOT NULL,
		scopes      TEXT NOT NULL DEFAULT '[]',
		created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at  DATETIME,
		last_used_at DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Put stores or updates an encrypted secret.
func (s *Store) Put(name string, encryptedValue, encryptedDataKey []byte, labels map[string]string) error {
	if labels == nil {
		labels = make(map[string]string)
	}
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return fmt.Errorf("marshaling labels: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO secrets (name, encrypted_value, encrypted_data_key, labels, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET
			encrypted_value    = excluded.encrypted_value,
			encrypted_data_key = excluded.encrypted_data_key,
			labels             = excluded.labels,
			updated_at         = CURRENT_TIMESTAMP
	`, name, encryptedValue, encryptedDataKey, string(labelsJSON))
	if err != nil {
		return fmt.Errorf("storing secret %q: %w", name, err)
	}
	return nil
}

// Get retrieves an encrypted secret by name.
func (s *Store) Get(name string) (*Secret, error) {
	var secret Secret
	var labelsJSON string

	err := s.db.QueryRow(`
		SELECT id, name, encrypted_value, encrypted_data_key, labels, created_at, updated_at
		FROM secrets WHERE name = ?
	`, name).Scan(
		&secret.ID,
		&secret.Name,
		&secret.EncryptedValue,
		&secret.EncryptedDataKey,
		&labelsJSON,
		&secret.CreatedAt,
		&secret.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("secret %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("querying secret %q: %w", name, err)
	}

	if err := json.Unmarshal([]byte(labelsJSON), &secret.Labels); err != nil {
		return nil, fmt.Errorf("unmarshaling labels: %w", err)
	}

	return &secret, nil
}

// List returns all stored secrets ordered by name.
func (s *Store) List() ([]Secret, error) {
	rows, err := s.db.Query(`
		SELECT id, name, encrypted_value, encrypted_data_key, labels, created_at, updated_at
		FROM secrets ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}
	defer rows.Close()
	return scanSecrets(rows)
}

// Delete removes a secret by name.
func (s *Store) Delete(name string) error {
	result, err := s.db.Exec("DELETE FROM secrets WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("deleting secret %q: %w", name, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking affected rows: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("secret %q not found", name)
	}
	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database handle for sharing with other components.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Search returns secrets whose name or label values match the query string.
// Uses case-insensitive LIKE matching.
func (s *Store) Search(query string) ([]Secret, error) {
	pattern := "%" + query + "%"
	rows, err := s.db.Query(`
		SELECT id, name, encrypted_value, encrypted_data_key, labels, created_at, updated_at
		FROM secrets
		WHERE name LIKE ? COLLATE NOCASE
		   OR labels LIKE ? COLLATE NOCASE
		ORDER BY name
	`, pattern, pattern)
	if err != nil {
		return nil, fmt.Errorf("searching secrets: %w", err)
	}
	defer rows.Close()
	return scanSecrets(rows)
}

// ListByLabel returns secrets that have a label matching the given key and value.
// If value is empty, matches any secret that has the given key regardless of value.
func (s *Store) ListByLabel(key, value string) ([]Secret, error) {
	var rows *sql.Rows
	var err error

	if value == "" {
		// Match any secret with this label key present.
		pattern := fmt.Sprintf("%%\"%s\"%%", key)
		rows, err = s.db.Query(`
			SELECT id, name, encrypted_value, encrypted_data_key, labels, created_at, updated_at
			FROM secrets
			WHERE labels LIKE ?
			ORDER BY name
		`, pattern)
	} else {
		// Match exact key=value pair.
		pattern := fmt.Sprintf("%%\"%s\":\"%s\"%%", key, value)
		rows, err = s.db.Query(`
			SELECT id, name, encrypted_value, encrypted_data_key, labels, created_at, updated_at
			FROM secrets
			WHERE labels LIKE ?
			ORDER BY name
		`, pattern)
	}
	if err != nil {
		return nil, fmt.Errorf("filtering by label: %w", err)
	}
	defer rows.Close()
	return scanSecrets(rows)
}

// Count returns the total number of secrets in the store.
func (s *Store) Count() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM secrets").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting secrets: %w", err)
	}
	return count, nil
}

// Names returns a sorted list of all secret names. Useful for shell completions.
func (s *Store) Names() ([]string, error) {
	rows, err := s.db.Query("SELECT name FROM secrets ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("listing secret names: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scanning name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// scanSecrets reads Secret structs from sql.Rows.
func scanSecrets(rows *sql.Rows) ([]Secret, error) {
	var secrets []Secret
	for rows.Next() {
		var secret Secret
		var labelsJSON string
		if err := rows.Scan(
			&secret.ID,
			&secret.Name,
			&secret.EncryptedValue,
			&secret.EncryptedDataKey,
			&labelsJSON,
			&secret.CreatedAt,
			&secret.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}
		if err := json.Unmarshal([]byte(labelsJSON), &secret.Labels); err != nil {
			return nil, fmt.Errorf("unmarshaling labels: %w", err)
		}
		secrets = append(secrets, secret)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}
	return secrets, nil
}
