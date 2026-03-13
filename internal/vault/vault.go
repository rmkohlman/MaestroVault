// Package vault provides the high-level orchestration layer for MaestroVault.
// It ties together crypto, keychain, and store to provide a clean API for
// secrets management with envelope encryption.
package vault

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rmkohlman/MaestroVault/internal/crypto"
	"github.com/rmkohlman/MaestroVault/internal/keychain"
	"github.com/rmkohlman/MaestroVault/internal/store"
	"github.com/rmkohlman/MaestroVault/internal/touchid"
)

const (
	defaultDBName = "vault.db"
	vaultDirName  = ".maestrovault"
)

// SecretEntry represents a decrypted secret returned to callers.
type SecretEntry struct {
	Name      string
	Value     string
	Labels    map[string]string
	CreatedAt string
	UpdatedAt string
}

// Vault provides high-level operations for secrets management.
type Vault struct {
	store *store.Store
}

// Dir returns the default vault directory path (~/.maestrovault).
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", vaultDirName)
	}
	return filepath.Join(home, vaultDirName)
}

// DBPath returns the full path to the vault database file.
func DBPath() string {
	return filepath.Join(Dir(), defaultDBName)
}

// Init initializes a new vault: creates the vault directory, generates a
// master key, stores it in the macOS Keychain, and creates the SQLite database.
func Init() error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}

	if keychain.MasterKeyExists() {
		return fmt.Errorf("vault already initialized (master key exists in Keychain)")
	}

	masterKey, err := crypto.GenerateKey()
	if err != nil {
		return fmt.Errorf("generating master key: %w", err)
	}

	if err := keychain.StoreMasterKey(masterKey); err != nil {
		return fmt.Errorf("storing master key: %w", err)
	}

	// Verify the database can be created.
	dbPath := filepath.Join(dir, defaultDBName)
	s, err := store.New(dbPath)
	if err != nil {
		// Roll back: remove master key on failure.
		_ = keychain.DeleteMasterKey()
		return fmt.Errorf("creating database: %w", err)
	}
	s.Close()

	return nil
}

// Open opens an existing vault for use.
// If TouchID is enabled in the config, the user is prompted for biometric
// authentication before the vault is unlocked.
func Open() (*Vault, error) {
	if !keychain.MasterKeyExists() {
		return nil, fmt.Errorf("vault not initialized (run 'maestrovault init' first)")
	}

	// Check TouchID requirement.
	cfg, err := LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	if cfg.TouchID {
		if err := touchid.Authenticate("MaestroVault wants to access your secrets"); err != nil {
			return nil, fmt.Errorf("authentication required: %w", err)
		}
	}

	dbPath := DBPath()
	s, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	return &Vault{store: s}, nil
}

// Close closes the vault and releases database resources.
func (v *Vault) Close() error {
	return v.store.Close()
}

// DB returns the underlying database handle for sharing with the API server.
func (v *Vault) DB() *sql.DB {
	return v.store.DB()
}

// Set encrypts and stores a secret using envelope encryption.
// Each secret gets its own randomly generated data key.
func (v *Vault) Set(name, value string, labels map[string]string) error {
	if name == "" {
		return fmt.Errorf("secret name cannot be empty")
	}
	if labels == nil {
		labels = make(map[string]string)
	}

	masterKey, err := keychain.RetrieveMasterKey()
	if err != nil {
		return fmt.Errorf("retrieving master key: %w", err)
	}

	// Generate a unique data key for this secret.
	dataKey, err := crypto.GenerateKey()
	if err != nil {
		return fmt.Errorf("generating data key: %w", err)
	}

	// Encrypt the secret value with the data key.
	encryptedValue, err := crypto.Encrypt([]byte(value), dataKey)
	if err != nil {
		return fmt.Errorf("encrypting secret: %w", err)
	}

	// Encrypt the data key with the master key (envelope encryption).
	encryptedDataKey, err := crypto.EncryptDataKey(dataKey, masterKey)
	if err != nil {
		return fmt.Errorf("encrypting data key: %w", err)
	}

	if err := v.store.Put(name, encryptedValue, encryptedDataKey, labels); err != nil {
		return fmt.Errorf("storing secret: %w", err)
	}

	return nil
}

// Get retrieves and decrypts a secret by name.
func (v *Vault) Get(name string) (*SecretEntry, error) {
	secret, err := v.store.Get(name)
	if err != nil {
		return nil, err
	}

	masterKey, err := keychain.RetrieveMasterKey()
	if err != nil {
		return nil, fmt.Errorf("retrieving master key: %w", err)
	}

	// Decrypt the data key with the master key.
	dataKey, err := crypto.DecryptDataKey(secret.EncryptedDataKey, masterKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting data key: %w", err)
	}

	// Decrypt the secret value with the data key.
	plaintext, err := crypto.Decrypt(secret.EncryptedValue, dataKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting secret: %w", err)
	}

	return &SecretEntry{
		Name:      secret.Name,
		Value:     string(plaintext),
		Labels:    secret.Labels,
		CreatedAt: secret.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt: secret.UpdatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

// List returns metadata for all stored secrets (values are not decrypted).
func (v *Vault) List() ([]SecretEntry, error) {
	secrets, err := v.store.List()
	if err != nil {
		return nil, err
	}

	entries := make([]SecretEntry, len(secrets))
	for i, s := range secrets {
		entries[i] = SecretEntry{
			Name:      s.Name,
			Labels:    s.Labels,
			CreatedAt: s.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: s.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}
	return entries, nil
}

// Delete removes a secret from the vault.
func (v *Vault) Delete(name string) error {
	return v.store.Delete(name)
}

// Destroy removes the vault completely: database file and master key.
func Destroy() error {
	dbPath := DBPath()

	// Remove database and associated WAL/SHM files.
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		_ = os.Remove(dbPath + suffix)
	}

	// Remove vault directory if empty.
	_ = os.Remove(Dir())

	if err := keychain.DeleteMasterKey(); err != nil {
		return fmt.Errorf("removing master key from keychain: %w", err)
	}

	return nil
}

// Search returns secrets whose name or labels match the query.
func (v *Vault) Search(query string) ([]SecretEntry, error) {
	secrets, err := v.store.Search(query)
	if err != nil {
		return nil, err
	}
	return toEntries(secrets), nil
}

// ListByLabel returns secrets matching a label key and optional value.
func (v *Vault) ListByLabel(key, value string) ([]SecretEntry, error) {
	secrets, err := v.store.ListByLabel(key, value)
	if err != nil {
		return nil, err
	}
	return toEntries(secrets), nil
}

// Edit updates an existing secret's value and/or labels.
// Pass empty string for value to keep the existing value.
// Pass nil for labels to keep the existing labels.
func (v *Vault) Edit(name, newValue string, newLabels map[string]string) error {
	if name == "" {
		return fmt.Errorf("secret name cannot be empty")
	}

	// Retrieve the existing secret to preserve unchanged fields.
	existing, err := v.Get(name)
	if err != nil {
		return fmt.Errorf("retrieving existing secret: %w", err)
	}

	value := existing.Value
	if newValue != "" {
		value = newValue
	}

	labels := existing.Labels
	if newLabels != nil {
		labels = newLabels
	}

	return v.Set(name, value, labels)
}

// Generate creates a random password using the specified options and
// optionally stores it as a secret.
func (v *Vault) Generate(name string, opts crypto.GenerateOpts, labels map[string]string) (string, error) {
	password, err := crypto.GeneratePassword(opts)
	if err != nil {
		return "", fmt.Errorf("generating password: %w", err)
	}

	if name != "" {
		if err := v.Set(name, password, labels); err != nil {
			return "", fmt.Errorf("storing generated secret: %w", err)
		}
	}

	return password, nil
}

// ExportEntry represents a single secret for export/import.
type ExportEntry struct {
	Name   string            `json:"name"`
	Value  string            `json:"value"`
	Labels map[string]string `json:"labels,omitempty"`
}

// Export decrypts and returns all secrets for export.
func (v *Vault) Export() ([]ExportEntry, error) {
	secrets, err := v.store.List()
	if err != nil {
		return nil, err
	}

	masterKey, err := keychain.RetrieveMasterKey()
	if err != nil {
		return nil, fmt.Errorf("retrieving master key: %w", err)
	}

	entries := make([]ExportEntry, 0, len(secrets))
	for _, s := range secrets {
		dataKey, err := crypto.DecryptDataKey(s.EncryptedDataKey, masterKey)
		if err != nil {
			return nil, fmt.Errorf("decrypting data key for %q: %w", s.Name, err)
		}
		plaintext, err := crypto.Decrypt(s.EncryptedValue, dataKey)
		if err != nil {
			return nil, fmt.Errorf("decrypting secret %q: %w", s.Name, err)
		}
		entries = append(entries, ExportEntry{
			Name:   s.Name,
			Value:  string(plaintext),
			Labels: s.Labels,
		})
	}

	return entries, nil
}

// Import encrypts and stores a batch of secrets from an export.
func (v *Vault) Import(entries []ExportEntry) (int, error) {
	imported := 0
	for _, e := range entries {
		if e.Name == "" {
			continue
		}
		if err := v.Set(e.Name, e.Value, e.Labels); err != nil {
			return imported, fmt.Errorf("importing secret %q: %w", e.Name, err)
		}
		imported++
	}
	return imported, nil
}

// ExportJSON returns all secrets as a JSON byte slice.
func (v *Vault) ExportJSON() ([]byte, error) {
	entries, err := v.Export()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(entries, "", "  ")
}

// ImportJSON imports secrets from a JSON byte slice.
func (v *Vault) ImportJSON(data []byte) (int, error) {
	var entries []ExportEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return 0, fmt.Errorf("parsing import data: %w", err)
	}
	return v.Import(entries)
}

// VaultInfo contains metadata about the vault.
type VaultInfo struct {
	Dir         string
	DBPath      string
	DBSize      int64
	SecretCount int
}

// Info returns metadata about the vault.
func (v *Vault) Info() (*VaultInfo, error) {
	count, err := v.store.Count()
	if err != nil {
		return nil, err
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

// Names returns a sorted list of all secret names.
func (v *Vault) Names() ([]string, error) {
	return v.store.Names()
}

// Count returns the total number of secrets in the vault.
func (v *Vault) Count() (int, error) {
	return v.store.Count()
}

// toEntries converts store Secrets to SecretEntry metadata (no values decrypted).
func toEntries(secrets []store.Secret) []SecretEntry {
	entries := make([]SecretEntry, len(secrets))
	for i, s := range secrets {
		entries[i] = SecretEntry{
			Name:      s.Name,
			Labels:    s.Labels,
			CreatedAt: s.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: s.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}
	return entries
}
