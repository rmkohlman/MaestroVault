package crypto

import (
	"bytes"
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
