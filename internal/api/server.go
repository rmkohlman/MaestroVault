package api

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rmkohlman/MaestroVault/internal/vault"
)

const (
	// DefaultSocketName is the Unix socket filename inside the vault directory.
	DefaultSocketName = "maestrovault.sock"
)

// Server is the MaestroVault REST API server.
type Server struct {
	vault    *vault.Vault
	tokens   *TokenStore
	listener net.Listener
	http     *http.Server
	sockPath string
}

// ServerOpts configures the API server.
type ServerOpts struct {
	// SocketPath overrides the default socket path.
	// Default: ~/.maestrovault/maestrovault.sock
	SocketPath string

	// DB is the open database handle (shared with the vault's store).
	DB *sql.DB
}

// NewServer creates a new API server backed by the given vault.
func NewServer(v *vault.Vault, opts ServerOpts) (*Server, error) {
	sockPath := opts.SocketPath
	if sockPath == "" {
		sockPath = filepath.Join(vault.Dir(), DefaultSocketName)
	}

	if opts.DB == nil {
		return nil, fmt.Errorf("database handle is required")
	}

	tokens := NewTokenStore(opts.DB)

	s := &Server{
		vault:    v,
		tokens:   tokens,
		sockPath: sockPath,
	}

	mux := s.buildMux()

	s.http = &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s, nil
}

// SocketPath returns the path to the Unix socket.
func (s *Server) SocketPath() string {
	return s.sockPath
}

// TokenStore returns the server's token store (for CLI token management).
func (s *Server) TokenStore() *TokenStore {
	return s.tokens
}

// Start begins listening on the Unix socket and serving requests.
// It blocks until the server is shut down.
func (s *Server) Start() error {
	// Remove stale socket file.
	if err := os.Remove(s.sockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing stale socket: %w", err)
	}

	ln, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.sockPath, err)
	}
	s.listener = ln

	// Restrict socket permissions to owner only.
	if err := os.Chmod(s.sockPath, 0600); err != nil {
		ln.Close()
		return fmt.Errorf("setting socket permissions: %w", err)
	}

	log.Printf("API server listening on %s", s.sockPath)

	// Handle graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if err := s.http.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case sig := <-stop:
		log.Printf("Received %s, shutting down...", sig)
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	return s.Shutdown()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutting down server: %w", err)
	}

	// Clean up socket file.
	_ = os.Remove(s.sockPath)

	log.Println("API server stopped.")
	return nil
}

// buildMux constructs the HTTP router with all routes and middleware.
func (s *Server) buildMux() http.Handler {
	mux := http.NewServeMux()

	// Health check — no auth required.
	mux.HandleFunc("GET /v1/health", s.handleHealth)

	// All other routes require auth.
	authed := http.NewServeMux()

	// Secrets CRUD.
	authed.Handle("GET /v1/secrets", requireScope(ScopeRead)(http.HandlerFunc(s.handleListSecrets)))
	authed.Handle("GET /v1/secrets/{name}", requireScope(ScopeRead)(http.HandlerFunc(s.handleGetSecret)))
	authed.Handle("PUT /v1/secrets/{name}", requireScope(ScopeWrite)(http.HandlerFunc(s.handleSetSecret)))
	authed.Handle("PATCH /v1/secrets/{name}", requireScope(ScopeWrite)(http.HandlerFunc(s.handleEditSecret)))
	authed.Handle("DELETE /v1/secrets/{name}", requireScope(ScopeWrite)(http.HandlerFunc(s.handleDeleteSecret)))

	// Search.
	authed.Handle("GET /v1/search", requireScope(ScopeRead)(http.HandlerFunc(s.handleSearch)))

	// Generate.
	authed.Handle("POST /v1/generate", requireScope(ScopeGenerate)(http.HandlerFunc(s.handleGenerate)))

	// Vault info.
	authed.Handle("GET /v1/info", requireScope(ScopeRead)(http.HandlerFunc(s.handleInfo)))

	// Token management.
	authed.Handle("GET /v1/tokens", requireScope(ScopeAdmin)(http.HandlerFunc(s.handleListTokens)))
	authed.Handle("POST /v1/tokens", requireScope(ScopeAdmin)(http.HandlerFunc(s.handleCreateToken)))
	authed.Handle("DELETE /v1/tokens/{id}", requireScope(ScopeAdmin)(http.HandlerFunc(s.handleRevokeToken)))

	// Wrap authed mux with auth middleware.
	mux.Handle("/v1/", s.authMiddleware(authed))

	// Apply request logging to everything.
	return requestLogger(mux)
}
