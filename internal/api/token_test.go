package api

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// testDB opens an in-memory SQLite database with the api_tokens table
// including the salt column for HMAC-SHA256 token hashing.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS api_tokens (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			token_hash   TEXT NOT NULL,
			salt         TEXT NOT NULL DEFAULT '',
			scopes       TEXT NOT NULL DEFAULT '[]',
			created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at   DATETIME,
			last_used_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash);
	`)
	if err != nil {
		t.Fatalf("creating schema: %v", err)
	}

	return db
}

func TestTokenCreateAndValidate(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	plaintext, tok, err := ts.Create("test-token", []Scope{ScopeRead, ScopeWrite}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if tok.Name != "test-token" {
		t.Errorf("name: want %q, got %q", "test-token", tok.Name)
	}
	if len(tok.Scopes) != 2 {
		t.Errorf("scopes: want 2, got %d", len(tok.Scopes))
	}
	if tok.ID == "" {
		t.Error("expected non-empty ID")
	}
	if len(plaintext) < 68 { // "mvt_" + 64 hex
		t.Errorf("plaintext too short: %d chars", len(plaintext))
	}
	if plaintext[:4] != "mvt_" {
		t.Errorf("token prefix: want %q, got %q", "mvt_", plaintext[:4])
	}

	// Validate the token.
	validated, err := ts.Validate(plaintext)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if validated.ID != tok.ID {
		t.Errorf("validated ID: want %q, got %q", tok.ID, validated.ID)
	}
	if validated.Name != tok.Name {
		t.Errorf("validated name: want %q, got %q", tok.Name, validated.Name)
	}
}

func TestTokenValidateInvalid(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	_, err := ts.Validate("mvt_0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestTokenValidateExpired(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	expired := time.Now().Add(-1 * time.Hour)
	plaintext, _, err := ts.Create("expired-token", []Scope{ScopeRead}, &expired)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = ts.Validate(plaintext)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestTokenList(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	_, _, _ = ts.Create("token-a", []Scope{ScopeRead}, nil)
	_, _, _ = ts.Create("token-b", []Scope{ScopeWrite}, nil)
	_, _, _ = ts.Create("token-c", []Scope{ScopeAdmin}, nil)

	tokens, err := ts.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}
}

func TestTokenListEmpty(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	tokens, err := ts.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tokens) != 0 {
		t.Fatalf("expected 0 tokens, got %d", len(tokens))
	}
}

func TestTokenRevoke(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	plaintext, tok, _ := ts.Create("revokable", []Scope{ScopeRead}, nil)

	if err := ts.Revoke(tok.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// Token should no longer validate.
	_, err := ts.Validate(plaintext)
	if err == nil {
		t.Fatal("expected error after revocation")
	}

	// List should be empty.
	tokens, _ := ts.List()
	if len(tokens) != 0 {
		t.Fatalf("expected 0 tokens after revoke, got %d", len(tokens))
	}
}

func TestTokenRevokeNotFound(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	err := ts.Revoke("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent token ID")
	}
}

func TestTokenRevokeAll(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	_, _, _ = ts.Create("a", []Scope{ScopeRead}, nil)
	_, _, _ = ts.Create("b", []Scope{ScopeWrite}, nil)

	n, err := ts.RevokeAll()
	if err != nil {
		t.Fatalf("RevokeAll: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 revoked, got %d", n)
	}

	tokens, _ := ts.List()
	if len(tokens) != 0 {
		t.Fatalf("expected 0 tokens, got %d", len(tokens))
	}
}

func TestTokenHasScope(t *testing.T) {
	tok := &Token{Scopes: []Scope{ScopeRead, ScopeWrite}}

	if !tok.HasScope(ScopeRead) {
		t.Error("should have read scope")
	}
	if !tok.HasScope(ScopeWrite) {
		t.Error("should have write scope")
	}
	if tok.HasScope(ScopeGenerate) {
		t.Error("should not have generate scope")
	}
	if tok.HasScope(ScopeAdmin) {
		t.Error("should not have admin scope")
	}
}

func TestTokenAdminGrantsAll(t *testing.T) {
	tok := &Token{Scopes: []Scope{ScopeAdmin}}

	if !tok.HasScope(ScopeRead) {
		t.Error("admin should grant read")
	}
	if !tok.HasScope(ScopeWrite) {
		t.Error("admin should grant write")
	}
	if !tok.HasScope(ScopeGenerate) {
		t.Error("admin should grant generate")
	}
	if !tok.HasScope(ScopeAdmin) {
		t.Error("admin should grant admin")
	}
}

func TestTokenCreateEmptyName(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	_, _, err := ts.Create("", []Scope{ScopeRead}, nil)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestTokenCreateNoScopes(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	_, _, err := ts.Create("test", nil, nil)
	if err == nil {
		t.Fatal("expected error for nil scopes")
	}

	_, _, err = ts.Create("test", []Scope{}, nil)
	if err == nil {
		t.Fatal("expected error for empty scopes")
	}
}

func TestValidScope(t *testing.T) {
	if !ValidScope("read") {
		t.Error("read should be valid")
	}
	if !ValidScope("write") {
		t.Error("write should be valid")
	}
	if !ValidScope("generate") {
		t.Error("generate should be valid")
	}
	if !ValidScope("admin") {
		t.Error("admin should be valid")
	}
	if ValidScope("delete") {
		t.Error("delete should not be valid")
	}
	if ValidScope("") {
		t.Error("empty should not be valid")
	}
}

// TestTokenLegacyValidation verifies that tokens stored with plain SHA-256
// (empty salt) can still be validated after the HMAC-SHA256 upgrade.
func TestTokenLegacyValidation(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	// Manually insert a legacy token (no salt, plain SHA-256 hash).
	plaintext := "mvt_legacy_test_token_0000000000000000000000000000000000000000"
	legacyHash := hashToken(plaintext)

	_, err := db.Exec(`
		INSERT INTO api_tokens (id, name, token_hash, salt, scopes, created_at)
		VALUES ('legacy-id', 'legacy-token', ?, '', '["read"]', CURRENT_TIMESTAMP)
	`, legacyHash)
	if err != nil {
		t.Fatalf("inserting legacy token: %v", err)
	}

	// Should validate using legacy SHA-256 path.
	tok, err := ts.Validate(plaintext)
	if err != nil {
		t.Fatalf("Validate legacy token: %v", err)
	}
	if tok.ID != "legacy-id" {
		t.Errorf("legacy token ID: want %q, got %q", "legacy-id", tok.ID)
	}
	if tok.Name != "legacy-token" {
		t.Errorf("legacy token name: want %q, got %q", "legacy-token", tok.Name)
	}
}

// TestHashTokenWithSalt verifies HMAC-SHA256 produces deterministic output.
func TestHashTokenWithSalt(t *testing.T) {
	salt := []byte("testsalt12345678")
	h1 := hashTokenWithSalt("mvt_test", salt)
	h2 := hashTokenWithSalt("mvt_test", salt)

	if h1 != h2 {
		t.Error("same input should produce same hash")
	}

	h3 := hashTokenWithSalt("mvt_other", salt)
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}

	h4 := hashTokenWithSalt("mvt_test", []byte("differentsalt!!!"))
	if h1 == h4 {
		t.Error("different salt should produce different hash")
	}
}
