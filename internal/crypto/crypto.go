// Package crypto provides AES-256-GCM encryption primitives and envelope
// encryption for MaestroVault. Each secret is encrypted with a randomly
// generated data key, and data keys are encrypted with the master key.
//
// The Provider interface is the contract for all encryption operations.
// AES256GCM is the concrete implementation.
//
// Password/passphrase generation lives in generate.go and is NOT part of
// the Provider interface — those are package-level utilities.
package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

const (
	// KeySize is the size of AES-256 keys in bytes.
	KeySize = 32
)

var (
	ErrCiphertextTooShort = errors.New("ciphertext too short")
	ErrInvalidKeySize     = errors.New("invalid key size: must be 32 bytes")
)

// ── Interface ────────────────────────────────────────────────

// Provider defines the contract for symmetric encryption operations.
// All methods accept a context for consistency across the codebase,
// even though the current AES-256-GCM implementation does not use it.
type Provider interface {
	// GenerateKey generates a cryptographically random 256-bit key.
	GenerateKey(ctx context.Context) ([]byte, error)

	// Encrypt encrypts plaintext using AES-256-GCM with the provided key.
	// The returned ciphertext has the nonce prepended: [nonce | ciphertext | tag].
	Encrypt(ctx context.Context, plaintext, key []byte) ([]byte, error)

	// Decrypt decrypts ciphertext produced by Encrypt.
	// Expects the nonce prepended: [nonce | ciphertext | tag].
	Decrypt(ctx context.Context, ciphertext, key []byte) ([]byte, error)

	// EncryptDataKey encrypts a data key with the master key (envelope encryption).
	EncryptDataKey(ctx context.Context, dataKey, masterKey []byte) ([]byte, error)

	// DecryptDataKey decrypts a data key with the master key (envelope encryption).
	DecryptDataKey(ctx context.Context, encryptedDataKey, masterKey []byte) ([]byte, error)
}

// ── Concrete implementation ──────────────────────────────────

// AES256GCM implements Provider using AES-256 in GCM mode.
type AES256GCM struct{}

// New returns a Provider backed by AES-256-GCM.
func New() Provider { return &AES256GCM{} }

// Compile-time assertion: *AES256GCM implements Provider.
var _ Provider = (*AES256GCM)(nil)

func (a *AES256GCM) GenerateKey(_ context.Context) ([]byte, error) {
	return GenerateKey()
}

func (a *AES256GCM) Encrypt(_ context.Context, plaintext, key []byte) ([]byte, error) {
	return Encrypt(plaintext, key)
}

func (a *AES256GCM) Decrypt(_ context.Context, ciphertext, key []byte) ([]byte, error) {
	return Decrypt(ciphertext, key)
}

func (a *AES256GCM) EncryptDataKey(_ context.Context, dataKey, masterKey []byte) ([]byte, error) {
	return EncryptDataKey(dataKey, masterKey)
}

func (a *AES256GCM) DecryptDataKey(_ context.Context, encryptedDataKey, masterKey []byte) ([]byte, error) {
	return DecryptDataKey(encryptedDataKey, masterKey)
}

// ── Package-level functions (backward compatibility) ─────────

// GenerateKey generates a cryptographically random 256-bit key.
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generating random key: %w", err)
	}
	return key, nil
}

// Encrypt encrypts plaintext using AES-256-GCM with the provided key.
// The returned ciphertext has the nonce prepended: [nonce | ciphertext | tag].
func Encrypt(plaintext, key []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	// Seal appends the encrypted data (with auth tag) after the nonce.
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext produced by Encrypt.
// Expects the nonce prepended to the ciphertext: [nonce | ciphertext | tag].
func Decrypt(ciphertext, key []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrCiphertextTooShort
	}

	nonce, ciphertextBody := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBody, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w", err)
	}

	return plaintext, nil
}

// EncryptDataKey encrypts a data key using the master key (envelope encryption).
func EncryptDataKey(dataKey, masterKey []byte) ([]byte, error) {
	return Encrypt(dataKey, masterKey)
}

// DecryptDataKey decrypts a data key using the master key (envelope encryption).
func DecryptDataKey(encryptedDataKey, masterKey []byte) ([]byte, error) {
	return Decrypt(encryptedDataKey, masterKey)
}
