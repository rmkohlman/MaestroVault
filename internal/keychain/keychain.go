// Package keychain provides macOS Keychain integration for storing and
// retrieving the MaestroVault master key. The master key never leaves
// the Keychain in plaintext except when actively encrypting/decrypting.
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
