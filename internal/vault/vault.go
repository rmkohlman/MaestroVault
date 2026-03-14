// Package vault provides the high-level orchestration layer for MaestroVault.
// It ties together crypto, keychain, and store to provide a clean API for
// secrets management with envelope encryption.
//
// The Vault interface is the contract for all vault operations.
// Service is the concrete implementation using dependency injection.
//
// Open(ctx) is the convenience constructor that creates concrete deps.
// NewService(s, c, kc) is the DI constructor for testing with mocks.
// Neither performs TouchID — callers handle auth before calling Open/NewService.
package vault

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rmkohlman/MaestroVault/internal/crypto"
	keychainpkg "github.com/rmkohlman/MaestroVault/internal/keychain"
	"github.com/rmkohlman/MaestroVault/internal/store"
)

// ── Type aliases (re-exported from dependencies) ─────────────

// NameEntry is re-exported from store for callers that don't import store directly.
type NameEntry = store.NameEntry

// GenerateOpts is re-exported from crypto for callers that don't import crypto directly.
type GenerateOpts = crypto.GenerateOpts

// ── Error re-exports ─────────────────────────────────────────

var (
	// ErrNotFound is returned when a secret does not exist.
	ErrNotFound = store.ErrNotFound

	// ErrAlreadyExists is returned on duplicate (name, environment) insert.
	ErrAlreadyExists = store.ErrAlreadyExists
)

// ── Models ───────────────────────────────────────────────────

// SecretEntry represents a decrypted secret returned to callers.
// Used by List, Get, Search, and ListByMetadata.
type SecretEntry struct {
	Name        string         `json:"name"`
	Environment string         `json:"environment"`
	Value       string         `json:"value,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

// ExportEntry represents a single secret for export/import operations.
type ExportEntry struct {
	Name        string         `json:"name"`
	Environment string         `json:"environment,omitempty"`
	Value       string         `json:"value"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// VaultInfo contains metadata about the vault.
type VaultInfo struct {
	Dir         string `json:"dir"`
	DBPath      string `json:"db_path"`
	DBSize      int64  `json:"db_size_bytes"`
	SecretCount int    `json:"secret_count"`
}

// ── Interface ────────────────────────────────────────────────

// Vault defines the contract for all high-level secrets management operations.
// All methods accept a context for cancellation and tracing.
// Environment scoping: env="" means "default" environment.
type Vault interface {
	// Set encrypts and stores a secret using envelope encryption.
	Set(ctx context.Context, name, env, value string, metadata map[string]any) error

	// Get retrieves and decrypts a secret by name and environment.
	Get(ctx context.Context, name, env string) (*SecretEntry, error)

	// List returns metadata for all secrets (values not decrypted).
	// env="" returns secrets from all environments.
	List(ctx context.Context, env string) ([]SecretEntry, error)

	// Delete removes a secret by name and environment.
	Delete(ctx context.Context, name, env string) error

	// Edit updates an existing secret's value and/or metadata.
	// Pass nil for newValue to keep the existing value.
	// Pass nil for newMetadata to keep the existing metadata.
	Edit(ctx context.Context, name, env string, newValue *string, newMetadata map[string]any) error

	// Search returns secrets whose name, environment, or metadata match the query.
	Search(ctx context.Context, query string) ([]SecretEntry, error)

	// ListByMetadata returns secrets matching a metadata key and optional value.
	ListByMetadata(ctx context.Context, key, value string) ([]SecretEntry, error)

	// Generate creates a random password and optionally stores it.
	Generate(ctx context.Context, name, env string, opts GenerateOpts, metadata map[string]any) (string, error)

	// Export decrypts and returns all secrets for export.
	Export(ctx context.Context) ([]ExportEntry, error)

	// Import encrypts and stores a batch of secrets from an export.
	Import(ctx context.Context, entries []ExportEntry) (int, error)

	// ExportJSON returns all secrets as a JSON byte slice.
	ExportJSON(ctx context.Context) ([]byte, error)

	// ImportJSON imports secrets from a JSON byte slice.
	ImportJSON(ctx context.Context, data []byte) (int, error)

	// Info returns metadata about the vault (path, size, count).
	Info(ctx context.Context) (*VaultInfo, error)

	// Names returns all (name, environment) pairs for shell completions.
	Names(ctx context.Context) ([]NameEntry, error)

	// Count returns the total number of secrets.
	Count(ctx context.Context) (int, error)

	// Close releases all resources.
	Close() error

	// DB returns the underlying database handle for components that
	// need direct access (e.g., API token store).
	DB() *sql.DB
}

// ── Concrete implementation ──────────────────────────────────

// Service implements Vault using injected dependencies.
// Use NewService() for testing with mocks, or Open() for production.
type Service struct {
	store    store.SecretStore
	crypto   crypto.Provider
	keychain keychainpkg.Provider
}

// NewService creates a Vault backed by the given dependencies.
// This is the DI constructor — use it in tests with mock implementations.
func NewService(s store.SecretStore, c crypto.Provider, kc keychainpkg.Provider) *Service {
	return &Service{
		store:    s,
		crypto:   c,
		keychain: kc,
	}
}

// Compile-time assertion: *Service implements Vault.
var _ Vault = (*Service)(nil)

// ── Service method implementations ───────────────────────────

// Set encrypts and stores a secret using envelope encryption.
func (svc *Service) Set(ctx context.Context, name, env, value string, metadata map[string]any) error {
	if name == "" {
		return errors.New("secret name cannot be empty")
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}

	// Retrieve master key from keychain.
	masterKey, err := svc.keychain.RetrieveMasterKey()
	if err != nil {
		return fmt.Errorf("retrieving master key: %w", err)
	}

	// Generate a unique data key for this secret.
	dataKey, err := svc.crypto.GenerateKey(ctx)
	if err != nil {
		return fmt.Errorf("generating data key: %w", err)
	}

	// Encrypt the secret value with the data key.
	encSecret, err := svc.crypto.Encrypt(ctx, []byte(value), dataKey)
	if err != nil {
		return fmt.Errorf("encrypting secret: %w", err)
	}

	// Encrypt the data key with the master key (envelope encryption).
	encDataKey, err := svc.crypto.EncryptDataKey(ctx, dataKey, masterKey)
	if err != nil {
		return fmt.Errorf("encrypting data key: %w", err)
	}

	// Marshal metadata to JSON.
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	// Store the encrypted secret.
	if err := svc.store.Put(ctx, name, env, encSecret, encDataKey, metadataJSON); err != nil {
		return fmt.Errorf("storing secret: %w", err)
	}

	return nil
}

// Get retrieves and decrypts a secret by name and environment.
func (svc *Service) Get(ctx context.Context, name, env string) (*SecretEntry, error) {
	// Fetch encrypted secret from store.
	secret, err := svc.store.Get(ctx, name, env)
	if err != nil {
		return nil, err
	}

	// Retrieve master key from keychain.
	masterKey, err := svc.keychain.RetrieveMasterKey()
	if err != nil {
		return nil, fmt.Errorf("retrieving master key: %w", err)
	}

	// Decrypt the data key using the master key.
	dataKey, err := svc.crypto.DecryptDataKey(ctx, secret.EncryptedDataKey, masterKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting data key: %w", err)
	}

	// Decrypt the secret value using the data key.
	plaintext, err := svc.crypto.Decrypt(ctx, secret.EncryptedSecret, dataKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting secret: %w", err)
	}

	// Parse metadata.
	metadata := parseMetadata(secret.Metadata)

	return &SecretEntry{
		Name:        secret.Name,
		Environment: secret.Environment,
		Value:       string(plaintext),
		Metadata:    metadata,
		CreatedAt:   secret.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   secret.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// List returns metadata for all secrets (values not decrypted).
func (svc *Service) List(ctx context.Context, env string) ([]SecretEntry, error) {
	secrets, err := svc.store.List(ctx, env)
	if err != nil {
		return nil, err
	}
	return svc.toEntries(secrets), nil
}

// Delete removes a secret by name and environment.
func (svc *Service) Delete(ctx context.Context, name, env string) error {
	return svc.store.Delete(ctx, name, env)
}

// Edit updates an existing secret's value and/or metadata.
func (svc *Service) Edit(ctx context.Context, name, env string, newValue *string, newMetadata map[string]any) error {
	// Fetch existing secret (decrypted).
	existing, err := svc.Get(ctx, name, env)
	if err != nil {
		return err
	}

	// Determine final value.
	finalValue := existing.Value
	if newValue != nil {
		finalValue = *newValue
	}

	// Determine final metadata.
	finalMetadata := existing.Metadata
	if newMetadata != nil {
		finalMetadata = newMetadata
	}

	// Re-encrypt and store.
	return svc.Set(ctx, name, env, finalValue, finalMetadata)
}

// Search returns secrets whose name, environment, or metadata match the query.
func (svc *Service) Search(ctx context.Context, query string) ([]SecretEntry, error) {
	secrets, err := svc.store.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	return svc.toEntries(secrets), nil
}

// ListByMetadata returns secrets matching a metadata key and optional value.
func (svc *Service) ListByMetadata(ctx context.Context, key, value string) ([]SecretEntry, error) {
	secrets, err := svc.store.ListByMetadata(ctx, key, value)
	if err != nil {
		return nil, err
	}
	return svc.toEntries(secrets), nil
}

// Generate creates a random password and optionally stores it.
func (svc *Service) Generate(ctx context.Context, name, env string, opts GenerateOpts, metadata map[string]any) (string, error) {
	password, err := crypto.GeneratePassword(opts)
	if err != nil {
		return "", fmt.Errorf("generating password: %w", err)
	}

	// If a name is provided, store the generated password.
	if name != "" {
		if err := svc.Set(ctx, name, env, password, metadata); err != nil {
			return "", err
		}
	}

	return password, nil
}

// Export decrypts and returns all secrets for export.
func (svc *Service) Export(ctx context.Context) ([]ExportEntry, error) {
	// Fetch all secrets (all environments).
	secrets, err := svc.store.List(ctx, "")
	if err != nil {
		return nil, err
	}

	// Retrieve master key once for bulk decryption.
	masterKey, err := svc.keychain.RetrieveMasterKey()
	if err != nil {
		return nil, fmt.Errorf("retrieving master key: %w", err)
	}

	entries := make([]ExportEntry, 0, len(secrets))
	for _, s := range secrets {
		// Decrypt data key.
		dataKey, err := svc.crypto.DecryptDataKey(ctx, s.EncryptedDataKey, masterKey)
		if err != nil {
			return nil, fmt.Errorf("decrypting data key for %q: %w", s.Name, err)
		}

		// Decrypt value.
		plaintext, err := svc.crypto.Decrypt(ctx, s.EncryptedSecret, dataKey)
		if err != nil {
			return nil, fmt.Errorf("decrypting secret %q: %w", s.Name, err)
		}

		metadata := parseMetadata(s.Metadata)

		entries = append(entries, ExportEntry{
			Name:        s.Name,
			Environment: s.Environment,
			Value:       string(plaintext),
			Metadata:    metadata,
		})
	}

	return entries, nil
}

// Import encrypts and stores a batch of secrets from an export.
func (svc *Service) Import(ctx context.Context, entries []ExportEntry) (int, error) {
	count := 0
	for _, entry := range entries {
		if entry.Name == "" {
			continue
		}
		if err := svc.Set(ctx, entry.Name, entry.Environment, entry.Value, entry.Metadata); err != nil {
			return count, fmt.Errorf("importing %q: %w", entry.Name, err)
		}
		count++
	}
	return count, nil
}

// ExportJSON returns all secrets as a JSON byte slice.
func (svc *Service) ExportJSON(ctx context.Context) ([]byte, error) {
	entries, err := svc.Export(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling export to JSON: %w", err)
	}
	return data, nil
}

// ImportJSON imports secrets from a JSON byte slice.
func (svc *Service) ImportJSON(ctx context.Context, data []byte) (int, error) {
	var entries []ExportEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return 0, fmt.Errorf("parsing import JSON: %w", err)
	}
	return svc.Import(ctx, entries)
}

// Info returns metadata about the vault (path, size, count).
func (svc *Service) Info(ctx context.Context) (*VaultInfo, error) {
	count, err := svc.store.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("counting secrets: %w", err)
	}

	dbPath := DBPath()
	var dbSize int64
	if fi, err := os.Stat(dbPath); err == nil {
		dbSize = fi.Size()
	}

	return &VaultInfo{
		Dir:         Dir(),
		DBPath:      dbPath,
		DBSize:      dbSize,
		SecretCount: count,
	}, nil
}

// Names returns all (name, environment) pairs for shell completions.
func (svc *Service) Names(ctx context.Context) ([]NameEntry, error) {
	return svc.store.Names(ctx)
}

// Count returns the total number of secrets.
func (svc *Service) Count(ctx context.Context) (int, error) {
	return svc.store.Count(ctx)
}

// Close releases all resources.
func (svc *Service) Close() error {
	return svc.store.Close()
}

// DB returns the underlying database handle.
func (svc *Service) DB() *sql.DB {
	return svc.store.DB()
}

// ── Helper methods ───────────────────────────────────────────

// toEntries converts store.Secret slice to SecretEntry slice without decryption.
// Value is left empty; metadata is parsed from JSON.
func (svc *Service) toEntries(secrets []store.Secret) []SecretEntry {
	entries := make([]SecretEntry, len(secrets))
	for i, s := range secrets {
		entries[i] = SecretEntry{
			Name:        s.Name,
			Environment: s.Environment,
			Metadata:    parseMetadata(s.Metadata),
			CreatedAt:   s.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   s.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return entries
}

// parseMetadata parses a json.RawMessage into a map[string]any.
// Returns an empty map on nil or invalid JSON.
func parseMetadata(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}
	}
	if m == nil {
		return map[string]any{}
	}
	return m
}

// ── Convenience functions ────────────────────────────────────

// Dir returns the default vault directory path (~/.maestrovault).
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback if home dir cannot be determined.
		return filepath.Join(os.TempDir(), ".maestrovault")
	}
	return filepath.Join(home, ".maestrovault")
}

// DBPath returns the full path to the vault database file.
func DBPath() string {
	return filepath.Join(Dir(), "vault.db")
}

// Open opens an existing vault using concrete dependencies (no TouchID).
// Callers are responsible for TouchID authentication before calling Open.
func Open(ctx context.Context) (Vault, error) {
	// Verify the master key exists before proceeding.
	if !keychainpkg.MasterKeyExists() {
		return nil, errors.New("vault not initialized: no master key found in keychain")
	}

	// Create concrete dependencies.
	s, err := store.New(DBPath())
	if err != nil {
		return nil, fmt.Errorf("opening store: %w", err)
	}

	c := crypto.New()
	kc := keychainpkg.New()

	return NewService(s, c, kc), nil
}

// Init initializes a new vault: creates directory, generates master key,
// stores it in keychain, and creates the database.
func Init(ctx context.Context) error {
	dir := Dir()

	// Create vault directory with restricted permissions.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}

	// Check if a master key already exists — refuse to overwrite.
	if keychainpkg.MasterKeyExists() {
		return errors.New("vault already initialized: master key exists in keychain")
	}

	// Generate a new master key.
	masterKey, err := crypto.GenerateKey()
	if err != nil {
		return fmt.Errorf("generating master key: %w", err)
	}

	// Store the master key in the keychain.
	if err := keychainpkg.StoreMasterKey(masterKey); err != nil {
		return fmt.Errorf("storing master key: %w", err)
	}

	// Create the database (this runs migrations).
	s, err := store.New(DBPath())
	if err != nil {
		// Rollback: remove master key on failure.
		_ = keychainpkg.DeleteMasterKey()
		return fmt.Errorf("creating database: %w", err)
	}
	_ = s.Close()

	return nil
}

// Destroy removes the vault completely: database file and master key.
func Destroy() error {
	dbPath := DBPath()

	// Remove the database file and related WAL/SHM/journal files.
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		_ = os.Remove(dbPath + suffix)
	}

	// Remove the vault directory if it's now empty.
	dir := Dir()
	_ = os.Remove(dir) // os.Remove only removes empty directories.

	// Delete the master key from the keychain.
	if err := keychainpkg.DeleteMasterKey(); err != nil {
		return fmt.Errorf("deleting master key: %w", err)
	}

	return nil
}
