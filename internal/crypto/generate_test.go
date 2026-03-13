package crypto

import (
	"strings"
	"testing"
)

func TestGeneratePasswordDefaults(t *testing.T) {
	pw, err := GeneratePassword(DefaultGenerateOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pw) != 32 {
		t.Errorf("expected length 32, got %d", len(pw))
	}
}

func TestGeneratePasswordLength(t *testing.T) {
	for _, length := range []int{8, 16, 64, 128} {
		opts := DefaultGenerateOpts()
		opts.Length = length
		pw, err := GeneratePassword(opts)
		if err != nil {
			t.Fatalf("length %d: unexpected error: %v", length, err)
		}
		if len(pw) != length {
			t.Errorf("expected length %d, got %d", length, len(pw))
		}
	}
}

func TestGeneratePasswordCharSets(t *testing.T) {
	opts := GenerateOpts{Length: 100, Lowercase: true, Uppercase: false, Digits: false, Symbols: false}
	pw, err := GeneratePassword(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range pw {
		if !strings.ContainsRune(lowercaseChars, c) {
			t.Errorf("unexpected char %q in lowercase-only password", c)
		}
	}

	opts = GenerateOpts{Length: 100, Lowercase: false, Uppercase: false, Digits: true, Symbols: false}
	pw, err = GeneratePassword(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range pw {
		if !strings.ContainsRune(digitChars, c) {
			t.Errorf("unexpected char %q in digits-only password", c)
		}
	}
}

func TestGeneratePasswordGuaranteesRequired(t *testing.T) {
	opts := GenerateOpts{Length: 4, Lowercase: true, Uppercase: true, Digits: true, Symbols: true}
	// Run multiple times to check guarantee holds.
	for i := 0; i < 50; i++ {
		pw, err := GeneratePassword(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		hasLower, hasUpper, hasDigit, hasSymbol := false, false, false, false
		for _, c := range pw {
			switch {
			case strings.ContainsRune(lowercaseChars, c):
				hasLower = true
			case strings.ContainsRune(uppercaseChars, c):
				hasUpper = true
			case strings.ContainsRune(digitChars, c):
				hasDigit = true
			case strings.ContainsRune(symbolChars, c):
				hasSymbol = true
			}
		}
		if !hasLower || !hasUpper || !hasDigit || !hasSymbol {
			t.Errorf("password %q missing required charset (lower=%v upper=%v digit=%v symbol=%v)",
				pw, hasLower, hasUpper, hasDigit, hasSymbol)
		}
	}
}

func TestGeneratePasswordUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		pw, err := GeneratePassword(DefaultGenerateOpts())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seen[pw] {
			t.Errorf("duplicate password generated: %q", pw)
		}
		seen[pw] = true
	}
}

func TestGeneratePasswordErrors(t *testing.T) {
	// Zero length.
	_, err := GeneratePassword(GenerateOpts{Length: 0, Lowercase: true})
	if err == nil {
		t.Error("expected error for zero length")
	}

	// No charsets.
	_, err = GeneratePassword(GenerateOpts{Length: 10})
	if err == nil {
		t.Error("expected error for no charsets")
	}

	// Length too short for required charsets.
	_, err = GeneratePassword(GenerateOpts{Length: 2, Lowercase: true, Uppercase: true, Digits: true})
	if err == nil {
		t.Error("expected error for length shorter than required charsets")
	}
}

func TestGeneratePassphrase(t *testing.T) {
	phrase, err := GeneratePassphrase(4, "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	words := strings.Split(phrase, "-")
	if len(words) != 4 {
		t.Errorf("expected 4 words, got %d: %q", len(words), phrase)
	}
}

func TestGeneratePassphraseUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		phrase, err := GeneratePassphrase(5, "-")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seen[phrase] {
			t.Errorf("duplicate passphrase: %q", phrase)
		}
		seen[phrase] = true
	}
}

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(token) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("expected 64 hex chars, got %d", len(token))
	}
}

func TestGenerateTokenError(t *testing.T) {
	_, err := GenerateToken(0)
	if err == nil {
		t.Error("expected error for zero byte length")
	}
}
