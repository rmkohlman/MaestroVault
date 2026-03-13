// Package client provides a Go client library for the MaestroVault REST API.
// It communicates over a Unix domain socket using standard HTTP/JSON.
//
// Usage:
//
//	c, err := client.New("mvt_abc123...")
//	if err != nil { ... }
//
//	secret, err := c.Get("my-secret")
//	entries, err := c.List()
//	err = c.Set("key", "value", nil)
//	err = c.Delete("key")
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultSocketName = "maestrovault.sock"
	vaultDirName      = ".maestrovault"
)

// Client is a Go client for the MaestroVault REST API.
type Client struct {
	http     *http.Client
	token    string
	baseURL  string
	sockPath string
}

// Option configures the client.
type Option func(*Client)

// WithSocketPath sets a custom Unix socket path.
func WithSocketPath(path string) Option {
	return func(c *Client) {
		c.sockPath = path
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.http.Timeout = d
	}
}

// New creates a new MaestroVault API client.
// The token is the plaintext API token (mvt_...).
func New(token string, opts ...Option) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("API token is required")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("finding home directory: %w", err)
	}
	sockPath := filepath.Join(home, vaultDirName, defaultSocketName)

	c := &Client{
		token:    token,
		sockPath: sockPath,
		baseURL:  "http://maestrovault",
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	// Configure Unix socket transport.
	c.http.Transport = &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", c.sockPath)
		},
	}

	return c, nil
}

// ── Secret types ──────────────────────────────────────────────

// SecretEntry represents a secret returned by the API.
type SecretEntry struct {
	Name      string            `json:"Name"`
	Value     string            `json:"Value,omitempty"`
	Labels    map[string]string `json:"Labels,omitempty"`
	CreatedAt string            `json:"CreatedAt,omitempty"`
	UpdatedAt string            `json:"UpdatedAt,omitempty"`
}

// VaultInfo contains metadata about the vault.
type VaultInfo struct {
	Dir         string `json:"dir"`
	DBPath      string `json:"db_path"`
	DBSize      int64  `json:"db_size_bytes"`
	SecretCount int    `json:"secret_count"`
}

// GenerateResult is the response from the generate endpoint.
type GenerateResult struct {
	Password string `json:"password"`
	Name     string `json:"name,omitempty"`
	Stored   bool   `json:"stored"`
}

// GenerateOpts configures password generation.
type GenerateOpts struct {
	Name      string            `json:"name,omitempty"`
	Length    int               `json:"length,omitempty"`
	Uppercase *bool             `json:"uppercase,omitempty"`
	Lowercase *bool             `json:"lowercase,omitempty"`
	Digits    *bool             `json:"digits,omitempty"`
	Symbols   *bool             `json:"symbols,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// TokenInfo represents an API token (without the plaintext).
type TokenInfo struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// CreateTokenResult is the response from creating a token.
type CreateTokenResult struct {
	Token     string     `json:"token"`
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// ── Secrets CRUD ──────────────────────────────────────────────

// Get retrieves a secret by name (with decrypted value).
func (c *Client) Get(name string) (*SecretEntry, error) {
	var entry SecretEntry
	if err := c.doJSON("GET", "/v1/secrets/"+name, nil, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// List returns metadata for all secrets.
func (c *Client) List() ([]SecretEntry, error) {
	var entries []SecretEntry
	if err := c.doJSON("GET", "/v1/secrets", nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// ListByLabel returns secrets matching a label key and optional value.
func (c *Client) ListByLabel(key, value string) ([]SecretEntry, error) {
	path := "/v1/secrets?label_key=" + key
	if value != "" {
		path += "&label_value=" + value
	}
	var entries []SecretEntry
	if err := c.doJSON("GET", path, nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// Set stores or updates a secret.
func (c *Client) Set(name, value string, labels map[string]string) error {
	body := map[string]interface{}{
		"value": value,
	}
	if labels != nil {
		body["labels"] = labels
	}
	return c.doJSON("PUT", "/v1/secrets/"+name, body, nil)
}

// Edit updates a secret's value and/or labels. Nil fields are preserved.
func (c *Client) Edit(name string, value *string, labels map[string]string) error {
	body := map[string]interface{}{}
	if value != nil {
		body["value"] = *value
	}
	if labels != nil {
		body["labels"] = labels
	}
	return c.doJSON("PATCH", "/v1/secrets/"+name, body, nil)
}

// Delete removes a secret.
func (c *Client) Delete(name string) error {
	return c.doJSON("DELETE", "/v1/secrets/"+name, nil, nil)
}

// ── Search ────────────────────────────────────────────────────

// Search returns secrets matching the query string.
func (c *Client) Search(query string) ([]SecretEntry, error) {
	var entries []SecretEntry
	if err := c.doJSON("GET", "/v1/search?q="+query, nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// ── Generate ──────────────────────────────────────────────────

// Generate creates a random password with optional storage.
func (c *Client) Generate(opts GenerateOpts) (*GenerateResult, error) {
	var result GenerateResult
	if err := c.doJSON("POST", "/v1/generate", opts, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ── Vault info ────────────────────────────────────────────────

// Info returns vault metadata.
func (c *Client) Info() (*VaultInfo, error) {
	var info VaultInfo
	if err := c.doJSON("GET", "/v1/info", nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ── Health ────────────────────────────────────────────────────

// Health checks if the API server is running.
func (c *Client) Health() error {
	return c.doJSON("GET", "/v1/health", nil, nil)
}

// ── Token management ──────────────────────────────────────────

// ListTokens returns all API tokens (requires admin scope).
func (c *Client) ListTokens() ([]TokenInfo, error) {
	var tokens []TokenInfo
	if err := c.doJSON("GET", "/v1/tokens", nil, &tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

// CreateToken creates a new API token (requires admin scope).
func (c *Client) CreateToken(name string, scopes []string, expiresIn string) (*CreateTokenResult, error) {
	body := map[string]interface{}{
		"name":   name,
		"scopes": scopes,
	}
	if expiresIn != "" {
		body["expires_in"] = expiresIn
	}
	var result CreateTokenResult
	if err := c.doJSON("POST", "/v1/tokens", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RevokeToken deletes a token by ID (requires admin scope).
func (c *Client) RevokeToken(id string) error {
	return c.doJSON("DELETE", "/v1/tokens/"+id, nil, nil)
}

// ── HTTP internals ────────────────────────────────────────────

// apiError is the standard error body from the server.
type apiError struct {
	Error string `json:"error"`
}

func (c *Client) doJSON(method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr apiError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("API error (%d): %s", resp.StatusCode, apiErr.Error)
		}
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}

	return nil
}
