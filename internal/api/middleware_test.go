package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddlewareMissingHeader(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	s := &Server{tokens: ts}
	handler := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/secrets", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: want %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	var body apiError
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error == "" {
		t.Error("expected error message in response")
	}
}

func TestAuthMiddlewareBadFormat(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	s := &Server{tokens: ts}
	handler := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/secrets", nil)
	req.Header.Set("Authorization", "Basic abc123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: want %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	s := &Server{tokens: ts}
	handler := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/secrets", nil)
	req.Header.Set("Authorization", "Bearer mvt_invalid")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: want %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestAuthMiddlewareValidToken(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	plaintext, _, _ := ts.Create("test", []Scope{ScopeRead}, nil)

	s := &Server{tokens: ts}
	var gotToken *Token
	handler := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = tokenFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/secrets", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: want %d, got %d", http.StatusOK, rec.Code)
	}
	if gotToken == nil {
		t.Fatal("expected token in context")
	}
	if gotToken.Name != "test" {
		t.Errorf("token name: want %q, got %q", "test", gotToken.Name)
	}
}

func TestRequireScopeAllowed(t *testing.T) {
	tok := &Token{Scopes: []Scope{ScopeRead, ScopeWrite}}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := requireScope(ScopeRead)(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(withToken(req.Context(), tok))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: want %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestRequireScopeDenied(t *testing.T) {
	tok := &Token{Scopes: []Scope{ScopeRead}}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := requireScope(ScopeWrite)(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(withToken(req.Context(), tok))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status: want %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestRequireScopeNoToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := requireScope(ScopeRead)(inner)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: want %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireScopeAdminGrantsAll(t *testing.T) {
	tok := &Token{Scopes: []Scope{ScopeAdmin}}

	scopes := []Scope{ScopeRead, ScopeWrite, ScopeGenerate, ScopeAdmin}
	for _, scope := range scopes {
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		handler := requireScope(scope)(inner)

		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(withToken(req.Context(), tok))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("admin should grant %s scope, got status %d", scope, rec.Code)
		}
	}
}

func TestHealthEndpoint(t *testing.T) {
	// Health doesn't need vault or tokens, just the handler.
	s := &Server{}

	req := httptest.NewRequest("GET", "/v1/health", nil)
	rec := httptest.NewRecorder()
	s.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: want %d, got %d", http.StatusOK, rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status: want %q, got %q", "ok", body["status"])
	}
	if body["time"] == "" {
		t.Error("expected non-empty time")
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusCreated, map[string]string{"hello": "world"})

	if rec.Code != http.StatusCreated {
		t.Errorf("status: want %d, got %d", http.StatusCreated, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: want %q, got %q", "application/json", ct)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body["hello"] != "world" {
		t.Errorf("body: want %q, got %q", "world", body["hello"])
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "something went wrong")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: want %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var body apiError
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body.Error != "something went wrong" {
		t.Errorf("error: want %q, got %q", "something went wrong", body.Error)
	}
}

func TestTokenListEndpoint(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	_, _, _ = ts.Create("t1", []Scope{ScopeRead}, nil)
	_, _, _ = ts.Create("t2", []Scope{ScopeWrite}, nil)

	s := &Server{tokens: ts}

	req := httptest.NewRequest("GET", "/v1/tokens", nil)
	rec := httptest.NewRecorder()
	s.handleListTokens(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: want %d, got %d", http.StatusOK, rec.Code)
	}

	var tokens []Token
	if err := json.Unmarshal(rec.Body.Bytes(), &tokens); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(tokens))
	}
}

func TestTokenListEndpointEmpty(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	s := &Server{tokens: ts}

	req := httptest.NewRequest("GET", "/v1/tokens", nil)
	rec := httptest.NewRecorder()
	s.handleListTokens(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: want %d, got %d", http.StatusOK, rec.Code)
	}

	var tokens []Token
	if err := json.Unmarshal(rec.Body.Bytes(), &tokens); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(tokens))
	}
}

func TestRevokeTokenEndpoint(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	_, tok, _ := ts.Create("doomed", []Scope{ScopeRead}, nil)

	s := &Server{tokens: ts}

	// Use Go 1.22+ path values via SetPathValue.
	req := httptest.NewRequest("DELETE", "/v1/tokens/"+tok.ID, nil)
	req.SetPathValue("id", tok.ID)
	rec := httptest.NewRecorder()
	s.handleRevokeToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: want %d, got %d", http.StatusOK, rec.Code)
	}

	// Verify it's gone.
	tokens, _ := ts.List()
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens after revoke, got %d", len(tokens))
	}
}

func TestRevokeTokenEndpointNotFound(t *testing.T) {
	db := testDB(t)
	ts := NewTokenStore(db)

	s := &Server{tokens: ts}

	req := httptest.NewRequest("DELETE", "/v1/tokens/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	rec := httptest.NewRecorder()
	s.handleRevokeToken(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: want %d, got %d", http.StatusNotFound, rec.Code)
	}
}
