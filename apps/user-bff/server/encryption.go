package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
)

// encryptionPrefix distinguishes encrypted values from legacy plaintext.
// The "v1" version tag allows future key rotation with a new prefix.
const encryptionPrefix = "enc:v1:"

// encryptTOTPSecret encrypts a TOTP secret using AES-256-GCM.
// Returns a string in the format "enc:v1:<base64(nonce+ciphertext+tag)>".
// The key must be exactly 32 bytes.
func encryptTOTPSecret(plaintext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize()) // 12 bytes for GCM
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Seal appends ciphertext+tag to nonce, so the result is nonce+ciphertext+tag
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)

	return encryptionPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptTOTPSecret delegates to the shared auth package.
// Kept for backward compatibility with existing tests.
func decryptTOTPSecret(stored string, key []byte) (string, error) {
	return auth.DecryptTOTPSecret(stored, key)
}

// parseEncryptionKey delegates to the shared auth package.
// Kept for backward compatibility with existing tests.
func parseEncryptionKey(hexKey string) ([]byte, error) {
	return auth.ParseTOTPEncryptionKey(hexKey)
}
