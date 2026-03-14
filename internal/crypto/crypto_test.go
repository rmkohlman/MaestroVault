package crypto

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}
	if len(key) != KeySize {
		t.Fatalf("expected key length %d, got %d", KeySize, len(key))
	}
}

func TestGenerateKeyUniqueness(t *testing.T) {
	key1, _ := GenerateKey()
	key2, _ := GenerateKey()
	if bytes.Equal(key1, key2) {
		t.Fatal("two generated keys must not be equal")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, _ := GenerateKey()
	plaintext := []byte("super-secret-database-password")

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	if bytes.Equal(plaintext, ciphertext) {
		t.Fatal("ciphertext must not equal plaintext")
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestEncryptEmptyPlaintext(t *testing.T) {
	key, _ := GenerateKey()
	plaintext := []byte{}

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("expected empty plaintext, got %q", decrypted)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1, _ := GenerateKey()
	key2, _ := GenerateKey()
	plaintext := []byte("secret")

	ciphertext, _ := Encrypt(plaintext, key1)

	_, err := Decrypt(ciphertext, key2)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key, _ := GenerateKey()
	plaintext := []byte("secret")

	ciphertext, _ := Encrypt(plaintext, key)

	// Flip the last byte
	ciphertext[len(ciphertext)-1] ^= 0xff

	_, err := Decrypt(ciphertext, key)
	if err == nil {
		t.Fatal("expected error decrypting tampered ciphertext")
	}
}

func TestDecryptTooShort(t *testing.T) {
	key, _ := GenerateKey()

	_, err := Decrypt([]byte("short"), key)
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}

func TestEncryptInvalidKeySize(t *testing.T) {
	_, err := Encrypt([]byte("data"), []byte("short-key"))
	if err == nil {
		t.Fatal("expected error for invalid key size")
	}
}

func TestDecryptInvalidKeySize(t *testing.T) {
	_, err := Decrypt([]byte("doesn't matter what this is if key is bad"), []byte("short-key"))
	if err == nil {
		t.Fatal("expected error for invalid key size")
	}
}

func TestEnvelopeEncryptionRoundTrip(t *testing.T) {
	masterKey, _ := GenerateKey()
	dataKey, _ := GenerateKey()

	encrypted, err := EncryptDataKey(dataKey, masterKey)
	if err != nil {
		t.Fatalf("EncryptDataKey() error: %v", err)
	}

	decrypted, err := DecryptDataKey(encrypted, masterKey)
	if err != nil {
		t.Fatalf("DecryptDataKey() error: %v", err)
	}

	if !bytes.Equal(dataKey, decrypted) {
		t.Fatal("decrypted data key does not match original")
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	key, _ := GenerateKey()
	plaintext := []byte("same data")

	ct1, _ := Encrypt(plaintext, key)
	ct2, _ := Encrypt(plaintext, key)

	if bytes.Equal(ct1, ct2) {
		t.Fatal("encrypting same plaintext twice must produce different ciphertexts (random nonce)")
	}
}

// ── Provider interface tests ─────────────────────────────────

func TestNewReturnsProvider(t *testing.T) {
	p := New()
	if p == nil {
		t.Fatal("New() returned nil")
	}
	// Verify the concrete type.
	if _, ok := p.(*AES256GCM); !ok {
		t.Fatalf("New() returned %T, want *AES256GCM", p)
	}
}

func TestProviderGenerateKey(t *testing.T) {
	p := New()
	ctx := context.Background()

	key, err := p.GenerateKey(ctx)
	if err != nil {
		t.Fatalf("Provider.GenerateKey() error: %v", err)
	}
	if len(key) != KeySize {
		t.Fatalf("expected key length %d, got %d", KeySize, len(key))
	}

	// Two keys should differ.
	key2, err := p.GenerateKey(ctx)
	if err != nil {
		t.Fatalf("Provider.GenerateKey() second call error: %v", err)
	}
	if bytes.Equal(key, key2) {
		t.Fatal("two generated keys must not be equal")
	}
}

func TestProviderEncryptDecryptRoundTrip(t *testing.T) {
	p := New()
	ctx := context.Background()

	key, _ := p.GenerateKey(ctx)
	plaintext := []byte("provider-interface-round-trip-test")

	ciphertext, err := p.Encrypt(ctx, plaintext, key)
	if err != nil {
		t.Fatalf("Provider.Encrypt() error: %v", err)
	}

	if bytes.Equal(plaintext, ciphertext) {
		t.Fatal("ciphertext must not equal plaintext")
	}

	decrypted, err := p.Decrypt(ctx, ciphertext, key)
	if err != nil {
		t.Fatalf("Provider.Decrypt() error: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestProviderEncryptDecryptDataKey(t *testing.T) {
	p := New()
	ctx := context.Background()

	masterKey, _ := p.GenerateKey(ctx)
	dataKey, _ := p.GenerateKey(ctx)

	encrypted, err := p.EncryptDataKey(ctx, dataKey, masterKey)
	if err != nil {
		t.Fatalf("Provider.EncryptDataKey() error: %v", err)
	}

	decrypted, err := p.DecryptDataKey(ctx, encrypted, masterKey)
	if err != nil {
		t.Fatalf("Provider.DecryptDataKey() error: %v", err)
	}

	if !bytes.Equal(dataKey, decrypted) {
		t.Fatal("decrypted data key does not match original")
	}
}

func TestProviderEncryptInvalidKeySize(t *testing.T) {
	p := New()
	ctx := context.Background()

	_, err := p.Encrypt(ctx, []byte("data"), []byte("short-key"))
	if err == nil {
		t.Fatal("expected error for invalid key size")
	}
	if !errors.Is(err, ErrInvalidKeySize) {
		t.Fatalf("expected ErrInvalidKeySize, got: %v", err)
	}
}

func TestProviderDecryptInvalidKeySize(t *testing.T) {
	p := New()
	ctx := context.Background()

	_, err := p.Decrypt(ctx, []byte("irrelevant"), []byte("short-key"))
	if err == nil {
		t.Fatal("expected error for invalid key size")
	}
	if !errors.Is(err, ErrInvalidKeySize) {
		t.Fatalf("expected ErrInvalidKeySize, got: %v", err)
	}
}

func TestProviderDecryptCiphertextTooShort(t *testing.T) {
	p := New()
	ctx := context.Background()

	key, _ := p.GenerateKey(ctx)
	_, err := p.Decrypt(ctx, []byte("short"), key)
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
	if !errors.Is(err, ErrCiphertextTooShort) {
		t.Fatalf("expected ErrCiphertextTooShort, got: %v", err)
	}
}

func TestProviderDecryptWrongKey(t *testing.T) {
	p := New()
	ctx := context.Background()

	key1, _ := p.GenerateKey(ctx)
	key2, _ := p.GenerateKey(ctx)
	plaintext := []byte("secret")

	ciphertext, _ := p.Encrypt(ctx, plaintext, key1)

	_, err := p.Decrypt(ctx, ciphertext, key2)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestProviderContextCancelled(t *testing.T) {
	// The AES256GCM implementation ignores ctx, but we verify
	// the interface accepts a cancelled context without panic.
	p := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// These should still succeed because AES256GCM doesn't check ctx.
	key, err := p.GenerateKey(ctx)
	if err != nil {
		t.Fatalf("GenerateKey with cancelled ctx error: %v", err)
	}
	ct, err := p.Encrypt(ctx, []byte("test"), key)
	if err != nil {
		t.Fatalf("Encrypt with cancelled ctx error: %v", err)
	}
	_, err = p.Decrypt(ctx, ct, key)
	if err != nil {
		t.Fatalf("Decrypt with cancelled ctx error: %v", err)
	}
}

// ── Backward compatibility: package-level functions still work ──

func TestBackwardCompatPackageFunctions(t *testing.T) {
	// This test explicitly exercises the package-level functions
	// (without context) to ensure they remain functional.
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}

	plaintext := []byte("backward-compat-test")

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if !bytes.Equal(plaintext, pt) {
		t.Fatalf("expected %q, got %q", plaintext, pt)
	}

	// Envelope encryption round-trip.
	masterKey, _ := GenerateKey()
	dataKey, _ := GenerateKey()

	enc, err := EncryptDataKey(dataKey, masterKey)
	if err != nil {
		t.Fatalf("EncryptDataKey() error: %v", err)
	}

	dec, err := DecryptDataKey(enc, masterKey)
	if err != nil {
		t.Fatalf("DecryptDataKey() error: %v", err)
	}

	if !bytes.Equal(dataKey, dec) {
		t.Fatal("decrypted data key does not match original")
	}
}

// ── Cross-compatibility: Provider and package-level are interchangeable ──

func TestProviderAndPackageFunctionsInteroperate(t *testing.T) {
	p := New()
	ctx := context.Background()

	key, _ := GenerateKey() // package-level
	plaintext := []byte("cross-compat-test")

	// Encrypt with Provider, decrypt with package-level.
	ct, err := p.Encrypt(ctx, plaintext, key)
	if err != nil {
		t.Fatalf("Provider.Encrypt() error: %v", err)
	}
	pt, err := Decrypt(ct, key) // package-level
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}
	if !bytes.Equal(plaintext, pt) {
		t.Fatal("Provider-encrypted data could not be decrypted by package-level function")
	}

	// Encrypt with package-level, decrypt with Provider.
	ct2, err := Encrypt(plaintext, key) // package-level
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}
	pt2, err := p.Decrypt(ctx, ct2, key)
	if err != nil {
		t.Fatalf("Provider.Decrypt() error: %v", err)
	}
	if !bytes.Equal(plaintext, pt2) {
		t.Fatal("Package-level-encrypted data could not be decrypted by Provider")
	}
}
