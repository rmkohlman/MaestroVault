// Package crypto provides AES-256-GCM encryption primitives and envelope
// encryption for MaestroVault. Each secret is encrypted with a randomly
// generated data key, and data keys are encrypted with the master key.
package crypto

import (
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
