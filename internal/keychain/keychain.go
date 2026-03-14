// Package keychain provides macOS Keychain integration for storing and
// retrieving the MaestroVault master key. The master key never leaves
// the Keychain in plaintext except when actively encrypting/decrypting.
//
// The Provider interface is the contract for all keychain operations.
// MacOSKeychain is the concrete implementation using github.com/keybase/go-keychain.
//
// Package-level functions are preserved for backward compatibility and
// delegate to a default MacOSKeychain instance.
package keychain

import (
	"fmt"

	gokeychain "github.com/keybase/go-keychain"
)

const (
	service = "MaestroVault"
	account = "master-key"
	label   = "MaestroVault Master Key"
)

// ── Interface ────────────────────────────────────────────────

// Provider defines the contract for master key storage.
// The master key is the root of the envelope encryption hierarchy.
type Provider interface {
	// StoreMasterKey stores the master key, replacing any existing one.
	StoreMasterKey(key []byte) error

	// RetrieveMasterKey retrieves the master key.
	RetrieveMasterKey() ([]byte, error)

	// DeleteMasterKey removes the master key.
	DeleteMasterKey() error

	// MasterKeyExists reports whether a master key is stored.
	MasterKeyExists() bool
}

// ── Concrete implementation ──────────────────────────────────

// MacOSKeychain implements Provider using the macOS Keychain via
// github.com/keybase/go-keychain.
type MacOSKeychain struct{}

// New returns a Provider backed by the macOS Keychain.
func New() Provider { return &MacOSKeychain{} }

// Compile-time assertion: *MacOSKeychain implements Provider.
var _ Provider = (*MacOSKeychain)(nil)

func (k *MacOSKeychain) StoreMasterKey(key []byte) error {
	return StoreMasterKey(key)
}

func (k *MacOSKeychain) RetrieveMasterKey() ([]byte, error) {
	return RetrieveMasterKey()
}

func (k *MacOSKeychain) DeleteMasterKey() error {
	return DeleteMasterKey()
}

func (k *MacOSKeychain) MasterKeyExists() bool {
	return MasterKeyExists()
}

// ── Package-level functions (backward compatibility) ─────────

// StoreMasterKey stores the master key in the macOS Keychain.
// If a key already exists, it is replaced.
func StoreMasterKey(key []byte) error {
	// Remove any existing entry first (ignore errors if not found).
	_ = DeleteMasterKey()

	item := gokeychain.NewItem()
	item.SetSecClass(gokeychain.SecClassGenericPassword)
	item.SetService(service)
	item.SetAccount(account)
	item.SetLabel(label)
	item.SetData(key)
	item.SetSynchronizable(gokeychain.SynchronizableNo)
	item.SetAccessible(gokeychain.AccessibleWhenUnlockedThisDeviceOnly)

	if err := gokeychain.AddItem(item); err != nil {
		return fmt.Errorf("storing master key in keychain: %w", err)
	}
	return nil
}

// RetrieveMasterKey retrieves the master key from the macOS Keychain.
func RetrieveMasterKey() ([]byte, error) {
	query := gokeychain.NewItem()
	query.SetSecClass(gokeychain.SecClassGenericPassword)
	query.SetService(service)
	query.SetAccount(account)
	query.SetMatchLimit(gokeychain.MatchLimitOne)
	query.SetReturnData(true)

	results, err := gokeychain.QueryItem(query)
	if err != nil {
		return nil, fmt.Errorf("querying keychain: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("master key not found in keychain")
	}

	return results[0].Data, nil
}

// DeleteMasterKey removes the master key from the macOS Keychain.
func DeleteMasterKey() error {
	item := gokeychain.NewItem()
	item.SetSecClass(gokeychain.SecClassGenericPassword)
	item.SetService(service)
	item.SetAccount(account)

	if err := gokeychain.DeleteItem(item); err != nil {
		return fmt.Errorf("deleting master key from keychain: %w", err)
	}
	return nil
}

// MasterKeyExists checks whether a master key is stored in the macOS Keychain.
func MasterKeyExists() bool {
	query := gokeychain.NewItem()
	query.SetSecClass(gokeychain.SecClassGenericPassword)
	query.SetService(service)
	query.SetAccount(account)
	query.SetMatchLimit(gokeychain.MatchLimitOne)
	query.SetReturnAttributes(true)

	results, err := gokeychain.QueryItem(query)
	if err != nil {
		return false
	}
	return len(results) > 0
}
