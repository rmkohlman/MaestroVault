package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/rmkohlman/MaestroVault/internal/crypto"
	"github.com/rmkohlman/MaestroVault/internal/vault"
)

// ── Context key for token ─────────────────────────────────────

type ctxKey string

const tokenCtxKey ctxKey = "api_token"

func withToken(ctx context.Context, tok *Token) context.Context {
	return context.WithValue(ctx, tokenCtxKey, tok)
}

func tokenFromContext(ctx context.Context) *Token {
	tok, _ := ctx.Value(tokenCtxKey).(*Token)
	return tok
}

// ── Error mapping ─────────────────────────────────────────────

// writeVaultError maps vault sentinel errors to appropriate HTTP status codes.
func writeVaultError(w http.ResponseWriter, err error) {
	if errors.Is(err, vault.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, vault.ErrAlreadyExists) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

// ── Health ────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// ── Secrets CRUD ──────────────────────────────────────────────

// GET /v1/secrets?env=&metadata_key=&metadata_value=
func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	env := r.URL.Query().Get("env")
	metadataKey := r.URL.Query().Get("metadata_key")
	metadataValue := r.URL.Query().Get("metadata_value")

	var entries []vault.SecretEntry
	var err error

	if metadataKey != "" {
		entries, err = s.vault.ListByMetadata(r.Context(), metadataKey, metadataValue)
	} else {
		entries, err = s.vault.List(r.Context(), env)
	}
	if err != nil {
		writeVaultError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, secretsToResponse(entries))
}

// GET /v1/secrets/{name}?env=
func (s *Server) handleGetSecret(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "secret name is required")
		return
	}

	env := r.URL.Query().Get("env")

	entry, err := s.vault.Get(r.Context(), name, env)
	if err != nil {
		writeVaultError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, secretToResponse(*entry))
}

// setSecretRequest is the body for PUT /v1/secrets/{name}.
type setSecretRequest struct {
	Value    string         `json:"value"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// PUT /v1/secrets/{name}?env=
func (s *Server) handleSetSecret(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "secret name is required")
		return
	}

	env := r.URL.Query().Get("env")

	var req setSecretRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Value == "" {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}

	if err := s.vault.Set(r.Context(), name, env, req.Value, req.Metadata); err != nil {
		writeVaultError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"status": "stored",
		"name":   name,
	})
}

// editSecretRequest is the body for PATCH /v1/secrets/{name}.
type editSecretRequest struct {
	Value    *string        `json:"value,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// PATCH /v1/secrets/{name}?env=
func (s *Server) handleEditSecret(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "secret name is required")
		return
	}

	env := r.URL.Query().Get("env")

	var req editSecretRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.vault.Edit(r.Context(), name, env, req.Value, req.Metadata); err != nil {
		writeVaultError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "updated",
		"name":   name,
	})
}

// DELETE /v1/secrets/{name}?env=
func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "secret name is required")
		return
	}

	env := r.URL.Query().Get("env")

	if err := s.vault.Delete(r.Context(), name, env); err != nil {
		writeVaultError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "deleted",
		"name":   name,
	})
}

// ── Search ────────────────────────────────────────────────────

// GET /v1/search?q=
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	entries, err := s.vault.Search(r.Context(), query)
	if err != nil {
		writeVaultError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, secretsToResponse(entries))
}

// ── Generate ──────────────────────────────────────────────────

// generateRequest is the body for POST /v1/generate.
type generateRequest struct {
	Name        string         `json:"name,omitempty"`
	Environment string         `json:"environment,omitempty"`
	Length      int            `json:"length,omitempty"`
	Uppercase   *bool          `json:"uppercase,omitempty"`
	Lowercase   *bool          `json:"lowercase,omitempty"`
	Digits      *bool          `json:"digits,omitempty"`
	Symbols     *bool          `json:"symbols,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type generateResponse struct {
	Password string `json:"password"`
	Name     string `json:"name,omitempty"`
	Stored   bool   `json:"stored"`
}

// POST /v1/generate
func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	opts := crypto.GenerateOpts{
		Length:    req.Length,
		Uppercase: boolDefault(req.Uppercase, true),
		Lowercase: boolDefault(req.Lowercase, true),
		Digits:    boolDefault(req.Digits, true),
		Symbols:   boolDefault(req.Symbols, true),
	}
	if opts.Length == 0 {
		opts.Length = 32
	}

	password, err := s.vault.Generate(r.Context(), req.Name, req.Environment, opts, req.Metadata)
	if err != nil {
		writeVaultError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, generateResponse{
		Password: password,
		Name:     req.Name,
		Stored:   req.Name != "",
	})
}

func boolDefault(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// ── Vault info ────────────────────────────────────────────────

// GET /v1/info
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.vault.Info(r.Context())
	if err != nil {
		writeVaultError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, info)
}

// ── Token management ──────────────────────────────────────────

type createTokenRequest struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresIn string   `json:"expires_in,omitempty"` // e.g. "24h", "30d", "0" for no expiry
}

type createTokenResponse struct {
	Token     string     `json:"token"` // plaintext, shown only once
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scopes    []Scope    `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// POST /v1/tokens
func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var req createTokenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	scopes := make([]Scope, 0, len(req.Scopes))
	for _, s := range req.Scopes {
		if !ValidScope(s) {
			writeError(w, http.StatusBadRequest, "invalid scope: "+s)
			return
		}
		scopes = append(scopes, Scope(s))
	}
	if len(scopes) == 0 {
		writeError(w, http.StatusBadRequest, "at least one scope is required")
		return
	}

	var expiresAt *time.Time
	if req.ExpiresIn != "" && req.ExpiresIn != "0" {
		d, err := time.ParseDuration(req.ExpiresIn)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid expires_in: "+err.Error())
			return
		}
		t := time.Now().Add(d)
		expiresAt = &t
	}

	plaintext, tok, err := s.tokens.Create(req.Name, scopes, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, createTokenResponse{
		Token:     plaintext,
		ID:        tok.ID,
		Name:      tok.Name,
		Scopes:    tok.Scopes,
		ExpiresAt: tok.ExpiresAt,
	})
}

// GET /v1/tokens
func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.tokens.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tokens == nil {
		tokens = []Token{}
	}
	writeJSON(w, http.StatusOK, tokens)
}

// DELETE /v1/tokens/{id}
func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "token id is required")
		return
	}

	if err := s.tokens.Revoke(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "revoked",
		"id":     id,
	})
}
