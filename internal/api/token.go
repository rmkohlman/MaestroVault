// Package api provides token management for the MaestroVault REST API.
// Tokens are stored as SHA-256 hashes — the plaintext token is only ever
// shown once at creation time.
package api

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Scope defines what an API token is allowed to do.
type Scope string

const (
	ScopeRead     Scope = "read"     // get, list, search, names, count, info
	ScopeWrite    Scope = "write"    // set, edit, delete, import
	ScopeGenerate Scope = "generate" // generate passwords
	ScopeAdmin    Scope = "admin"    // token management, export, destroy
)

// AllScopes is the complete set of scopes.
var AllScopes = []Scope{ScopeRead, ScopeWrite, ScopeGenerate, ScopeAdmin}

// ValidScope returns true if s is a recognized scope.
func ValidScope(s string) bool {
	switch Scope(s) {
	case ScopeRead, ScopeWrite, ScopeGenerate, ScopeAdmin:
		return true
	}
	return false
}

// Token represents a stored API token (without the plaintext).
type Token struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scopes     []Scope    `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// TokenStore manages API tokens in the database.
type TokenStore struct {
	db *sql.DB
}

// NewTokenStore creates a TokenStore backed by the given database.
func NewTokenStore(db *sql.DB) *TokenStore {
	return &TokenStore{db: db}
}

// Create generates a new API token, stores its hash, and returns the
// plaintext token. The plaintext is only available at creation time.
func (ts *TokenStore) Create(name string, scopes []Scope, expiresAt *time.Time) (plaintext string, tok *Token, err error) {
	if name == "" {
		return "", nil, fmt.Errorf("token name cannot be empty")
	}
	if len(scopes) == 0 {
		return "", nil, fmt.Errorf("at least one scope is required")
	}

	// Generate random token: 32 bytes = 64 hex chars.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generating token: %w", err)
	}
	plaintext = "mvt_" + hex.EncodeToString(raw)

	// Hash for storage.
	hash := hashToken(plaintext)

	// Generate an ID: 8 random hex bytes.
	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		return "", nil, fmt.Errorf("generating token id: %w", err)
	}
	id := hex.EncodeToString(idBytes)

	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return "", nil, fmt.Errorf("marshaling scopes: %w", err)
	}

	var expiresAtVal interface{}
	if expiresAt != nil {
		expiresAtVal = *expiresAt
	}

	_, err = ts.db.Exec(`
		INSERT INTO api_tokens (id, name, token_hash, scopes, created_at, expires_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, ?)
	`, id, name, hash, string(scopesJSON), expiresAtVal)
	if err != nil {
		return "", nil, fmt.Errorf("storing token: %w", err)
	}

	tok = &Token{
		ID:        id,
		Name:      name,
		Scopes:    scopes,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	return plaintext, tok, nil
}

// Validate checks a plaintext token against the database and returns the
// token record if valid. Returns an error if not found or expired.
func (ts *TokenStore) Validate(plaintext string) (*Token, error) {
	hash := hashToken(plaintext)

	var tok Token
	var scopesJSON string
	var expiresAt sql.NullTime
	var lastUsedAt sql.NullTime

	err := ts.db.QueryRow(`
		SELECT id, name, scopes, created_at, expires_at, last_used_at
		FROM api_tokens WHERE token_hash = ?
	`, hash).Scan(&tok.ID, &tok.Name, &scopesJSON, &tok.CreatedAt, &expiresAt, &lastUsedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid token")
	}
	if err != nil {
		return nil, fmt.Errorf("validating token: %w", err)
	}

	if err := json.Unmarshal([]byte(scopesJSON), &tok.Scopes); err != nil {
		return nil, fmt.Errorf("parsing token scopes: %w", err)
	}

	if expiresAt.Valid {
		tok.ExpiresAt = &expiresAt.Time
		if time.Now().After(expiresAt.Time) {
			return nil, fmt.Errorf("token expired")
		}
	}
	if lastUsedAt.Valid {
		tok.LastUsedAt = &lastUsedAt.Time
	}

	// Update last_used_at.
	go func() {
		_, _ = ts.db.Exec(`UPDATE api_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`, tok.ID)
	}()

	return &tok, nil
}

// HasScope returns true if the token has the given scope.
func (t *Token) HasScope(scope Scope) bool {
	for _, s := range t.Scopes {
		if s == scope || s == ScopeAdmin {
			return true
		}
	}
	return false
}

// List returns all stored tokens (without plaintext).
func (ts *TokenStore) List() ([]Token, error) {
	rows, err := ts.db.Query(`
		SELECT id, name, scopes, created_at, expires_at, last_used_at
		FROM api_tokens ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing tokens: %w", err)
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		var tok Token
		var scopesJSON string
		var expiresAt sql.NullTime
		var lastUsedAt sql.NullTime
		if err := rows.Scan(&tok.ID, &tok.Name, &scopesJSON, &tok.CreatedAt, &expiresAt, &lastUsedAt); err != nil {
			return nil, fmt.Errorf("scanning token: %w", err)
		}
		if err := json.Unmarshal([]byte(scopesJSON), &tok.Scopes); err != nil {
			return nil, fmt.Errorf("parsing scopes: %w", err)
		}
		if expiresAt.Valid {
			tok.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			tok.LastUsedAt = &lastUsedAt.Time
		}
		tokens = append(tokens, tok)
	}
	return tokens, rows.Err()
}

// Revoke deletes a token by ID.
func (ts *TokenStore) Revoke(id string) error {
	result, err := ts.db.Exec(`DELETE FROM api_tokens WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("revoking token: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("token %q not found", id)
	}
	return nil
}

// RevokeAll deletes all tokens.
func (ts *TokenStore) RevokeAll() (int, error) {
	result, err := ts.db.Exec(`DELETE FROM api_tokens`)
	if err != nil {
		return 0, fmt.Errorf("revoking all tokens: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// hashToken returns the SHA-256 hex digest of a plaintext token.
func hashToken(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}
