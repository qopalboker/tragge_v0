package server

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
)

func generateTestKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := generateTestKey(t)
	original := "JBSWY3DPEHPK3PXP" // sample base32 TOTP secret

	encrypted, err := encryptTOTPSecret(original, key)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	if !strings.HasPrefix(encrypted, encryptionPrefix) {
		t.Errorf("expected prefix %q, got %q", encryptionPrefix, encrypted[:len(encryptionPrefix)])
	}

	decrypted, err := decryptTOTPSecret(encrypted, key)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if decrypted != original {
		t.Errorf("round-trip mismatch: got %q, want %q", decrypted, original)
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	key := generateTestKey(t)
	secret := "JBSWY3DPEHPK3PXP"

	enc1, err := encryptTOTPSecret(secret, key)
	if err != nil {
		t.Fatal(err)
	}
	enc2, err := encryptTOTPSecret(secret, key)
	if err != nil {
		t.Fatal(err)
	}

	if enc1 == enc2 {
		t.Error("encrypting same plaintext twice should produce different ciphertexts (random nonce)")
	}
}

func TestDecryptLegacyPlaintext(t *testing.T) {
	key := generateTestKey(t)
	plaintext := "JBSWY3DPEHPK3PXP"

	result, err := decryptTOTPSecret(plaintext, key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != plaintext {
		t.Errorf("legacy plaintext not returned as-is: got %q, want %q", result, plaintext)
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	key1 := generateTestKey(t)
	key2 := generateTestKey(t)
	secret := "JBSWY3DPEHPK3PXP"

	encrypted, err := encryptTOTPSecret(secret, key1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = decryptTOTPSecret(encrypted, key2)
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestEncryptWrongKeyLength(t *testing.T) {
	shortKey := make([]byte, 16) // too short
	_, err := encryptTOTPSecret("test", shortKey)
	if err == nil {
		t.Error("expected error for wrong key length")
	}
}

func TestDecryptWrongKeyLength(t *testing.T) {
	key := generateTestKey(t)
	encrypted, err := encryptTOTPSecret("test", key)
	if err != nil {
		t.Fatal(err)
	}

	shortKey := make([]byte, 16)
	_, err = decryptTOTPSecret(encrypted, shortKey)
	if err == nil {
		t.Error("expected error for wrong key length on decrypt")
	}
}

func TestParseEncryptionKeyValid(t *testing.T) {
	original := generateTestKey(t)
	hexStr := hex.EncodeToString(original)

	key, err := parseEncryptionKey(hexStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(key))
	}
}

func TestParseEncryptionKeyTooShort(t *testing.T) {
	_, err := parseEncryptionKey("abcd")
	if err == nil {
		t.Error("expected error for short key")
	}
}

func TestParseEncryptionKeyInvalidHex(t *testing.T) {
	_, err := parseEncryptionKey("not-hex-at-all-this-is-invalid!!")
	if err == nil {
		t.Error("expected error for non-hex key")
	}
}
