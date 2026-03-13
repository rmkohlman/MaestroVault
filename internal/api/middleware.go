package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// ── Middleware ─────────────────────────────────────────────────

// authMiddleware validates the Bearer token and injects the Token into context.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			writeError(w, http.StatusUnauthorized, "missing Authorization header")
			return
		}

		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "invalid Authorization format (expected Bearer)")
			return
		}
		plaintext := strings.TrimPrefix(auth, "Bearer ")

		tok, err := s.tokens.Validate(plaintext)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}

		ctx := withToken(r.Context(), tok)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireScope returns middleware that checks the token has a required scope.
func requireScope(scope Scope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := tokenFromContext(r.Context())
			if tok == nil {
				writeError(w, http.StatusUnauthorized, "no token in context")
				return
			}
			if !tok.HasScope(scope) {
				writeError(w, http.StatusForbidden, fmt.Sprintf("scope %q required", scope))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requestLogger logs each request method + path.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("API %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// ── JSON helpers ──────────────────────────────────────────────

// apiError is the standard error response body.
type apiError struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiError{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
