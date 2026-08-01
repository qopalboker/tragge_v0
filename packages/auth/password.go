// Package auth provides authentication and authorization utilities for the trading platform.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2idParams holds the parameters for Argon2id password hashing.
type Argon2idParams struct {
	Memory      uint32 // Memory usage in KiB
	Iterations  uint32 // Number of iterations
	Parallelism uint8  // Degree of parallelism
	SaltLength  uint32 // Salt length in bytes
	KeyLength   uint32 // Derived key length in bytes
}

// DefaultArgon2idParams returns recommended parameters for Argon2id.
// These follow OWASP recommendations for password storage.
func DefaultArgon2idParams() *Argon2idParams {
	return &Argon2idParams{
		Memory:      64 * 1024, // 64 MiB
		Iterations:  3,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	}
}

var (
	// ErrInvalidHash indicates the encoded hash is not in the expected format.
	ErrInvalidHash = errors.New("auth: invalid password hash format")
	// ErrIncompatibleVersion indicates the Argon2 version is incompatible.
	ErrIncompatibleVersion = errors.New("auth: incompatible argon2 version")
	// ErrMismatchedPassword indicates the password does not match the hash.
	ErrMismatchedPassword = errors.New("auth: password does not match")

	// DummyHash is a pre-computed Argon2id hash used to equalize timing
	// during login when the user is not found. This prevents timing-based
	// user enumeration attacks by ensuring the response time is similar
	// whether or not the user exists.
	DummyHash string
)

func init() {
	DummyHash, _ = HashPassword("dummy-timing-equalization", nil)
}

// HashPassword hashes a password using Argon2id with the given parameters.
// If params is nil, DefaultArgon2idParams() is used.
// Returns the encoded hash in PHC string format.
func HashPassword(password string, params *Argon2idParams) (string, error) {
	if params == nil {
		params = DefaultArgon2idParams()
	}

	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: failed to generate salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	// Encode to PHC string format: $argon2id$v=19$m=65536,t=3,p=2$salt$hash
	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		params.Memory,
		params.Iterations,
		params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)

	return encoded, nil
}

// VerifyPassword verifies a password against an encoded Argon2id hash.
// Returns nil if the password matches, ErrMismatchedPassword if it doesn't,
// or another error if the hash is invalid.
func VerifyPassword(password, encodedHash string) error {
	params, salt, hash, err := decodeHash(encodedHash)
	if err != nil {
		return err
	}

	// Compute hash with the same parameters
	otherHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare(hash, otherHash) != 1 {
		return ErrMismatchedPassword
	}

	return nil
}

// decodeHash parses the encoded PHC string format and extracts parameters.
func decodeHash(encodedHash string) (*Argon2idParams, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return nil, nil, nil, ErrInvalidHash
	}

	if parts[1] != "argon2id" {
		return nil, nil, nil, ErrInvalidHash
	}

	var version int
	_, err := fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil {
		return nil, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
		return nil, nil, nil, ErrIncompatibleVersion
	}

	params := &Argon2idParams{}
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.Memory, &params.Iterations, &params.Parallelism)
	if err != nil {
		return nil, nil, nil, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, nil, ErrInvalidHash
	}
	params.SaltLength = uint32(len(salt))

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, ErrInvalidHash
	}
	params.KeyLength = uint32(len(hash))

	return params, salt, hash, nil
}

// NeedsRehash checks if a hash needs to be rehashed with new parameters.
// This is useful when upgrading security parameters over time.
func NeedsRehash(encodedHash string, params *Argon2idParams) bool {
	if params == nil {
		params = DefaultArgon2idParams()
	}

	currentParams, _, _, err := decodeHash(encodedHash)
	if err != nil {
		return true
	}

	return currentParams.Memory != params.Memory ||
		currentParams.Iterations != params.Iterations ||
		currentParams.Parallelism != params.Parallelism ||
		currentParams.KeyLength != params.KeyLength
}
