package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// encryptionPrefix distinguishes encrypted values from legacy plaintext.
// The "v1" version tag allows future key rotation with a new prefix.
const encryptionPrefix = "enc:v1:"

// VerifyTOTP verifies a TOTP code against a base32-encoded secret.
// Implements RFC 6238 with SHA1, 6 digits, 30-second period.
// Allows ±1 time step for clock drift tolerance.
func VerifyTOTP(secret, code string, now time.Time) bool {
	secretBytes, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return false
	}

	timeStep := now.Unix() / 30

	// Check current and adjacent time steps (±1) for clock drift
	for _, offset := range []int64{0, -1, 1} {
		counter := timeStep + offset
		generated := generateTOTPCode(secretBytes, counter)
		if subtle.ConstantTimeCompare([]byte(generated), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// generateTOTPCode generates a 6-digit TOTP code for a given counter.
func generateTOTPCode(secret []byte, counter int64) string {
	// Convert counter to big-endian 8-byte value
	buf := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		buf[i] = byte(counter & 0xff)
		counter >>= 8
	}

	// HMAC-SHA1
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf)
	hash := mac.Sum(nil)

	// Dynamic truncation (RFC 4226)
	offset := hash[len(hash)-1] & 0x0f
	truncated := (int64(hash[offset]&0x7f) << 24) |
		(int64(hash[offset+1]) << 16) |
		(int64(hash[offset+2]) << 8) |
		int64(hash[offset+3])

	// 6-digit code
	code := truncated % 1000000
	return fmt.Sprintf("%06d", code)
}

// DecryptTOTPSecret decrypts a TOTP secret encrypted with AES-256-GCM.
// If the value does not have the encryption prefix, it is returned as-is (legacy plaintext).
func DecryptTOTPSecret(stored string, key []byte) (string, error) {
	if !strings.HasPrefix(stored, encryptionPrefix) {
		return stored, nil
	}

	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}

	encoded := stored[len(encryptionPrefix):]
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintextBytes, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintextBytes), nil
}

// ParseTOTPEncryptionKey decodes a hex-encoded 32-byte encryption key.
func ParseTOTPEncryptionKey(hexKey string) ([]byte, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("TOTP_ENCRYPTION_KEY must be hex-encoded: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("TOTP_ENCRYPTION_KEY must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}
	return key, nil
}
