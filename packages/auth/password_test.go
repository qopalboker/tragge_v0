package auth

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	password := "mysecretpassword123"

	hash, err := HashPassword(password, nil)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	// Check hash format
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("Hash should start with $argon2id$, got: %s", hash[:20])
	}

	// Hash should have 6 parts when split by $
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Errorf("Hash should have 6 parts, got %d", len(parts))
	}
}

func TestHashPasswordUnique(t *testing.T) {
	password := "samepassword"

	hash1, _ := HashPassword(password, nil)
	hash2, _ := HashPassword(password, nil)

	// Two hashes of the same password should be different (different salts)
	if hash1 == hash2 {
		t.Error("Two hashes of the same password should be different")
	}
}

func TestVerifyPassword(t *testing.T) {
	password := "testpassword"

	hash, err := HashPassword(password, nil)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	// Correct password should verify
	if err := VerifyPassword(password, hash); err != nil {
		t.Errorf("VerifyPassword should succeed for correct password: %v", err)
	}
}

func TestVerifyPasswordWrong(t *testing.T) {
	password := "correctpassword"
	wrongPassword := "wrongpassword"

	hash, _ := HashPassword(password, nil)

	// Wrong password should fail
	err := VerifyPassword(wrongPassword, hash)
	if err == nil {
		t.Error("VerifyPassword should fail for wrong password")
	}
	if err != ErrMismatchedPassword {
		t.Errorf("Expected ErrMismatchedPassword, got: %v", err)
	}
}

func TestVerifyPasswordInvalidHash(t *testing.T) {
	testCases := []struct {
		name string
		hash string
	}{
		{"empty", ""},
		{"not enough parts", "$argon2id$v=19$m=65536"},
		{"wrong algorithm", "$argon2i$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA"},
		{"invalid base64 salt", "$argon2id$v=19$m=65536,t=3,p=2$!!!$aGFzaA"},
		{"invalid base64 hash", "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$!!!"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyPassword("password", tc.hash)
			if err == nil {
				t.Error("VerifyPassword should fail for invalid hash")
			}
		})
	}
}

func TestCustomParams(t *testing.T) {
	password := "testpassword"
	params := &Argon2idParams{
		Memory:      32 * 1024, // 32 MiB
		Iterations:  2,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}

	hash, err := HashPassword(password, params)
	if err != nil {
		t.Fatalf("HashPassword with custom params failed: %v", err)
	}

	// Should contain the custom memory parameter
	if !strings.Contains(hash, "m=32768") {
		t.Error("Hash should contain custom memory parameter")
	}

	// Should still verify correctly
	if err := VerifyPassword(password, hash); err != nil {
		t.Errorf("VerifyPassword should succeed: %v", err)
	}
}

func TestNeedsRehash(t *testing.T) {
	password := "testpassword"

	// Hash with low-cost params
	lowParams := &Argon2idParams{
		Memory:      32 * 1024,
		Iterations:  2,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
	hash, _ := HashPassword(password, lowParams)

	// Should need rehash with default params (higher cost)
	if !NeedsRehash(hash, nil) {
		t.Error("Should need rehash when current params differ from default")
	}

	// Should not need rehash with same params
	if NeedsRehash(hash, lowParams) {
		t.Error("Should not need rehash when params match")
	}
}

func TestNeedsRehashInvalidHash(t *testing.T) {
	// Invalid hash should need rehash
	if !NeedsRehash("invalid", nil) {
		t.Error("Invalid hash should need rehash")
	}
}

func BenchmarkHashPassword(b *testing.B) {
	password := "benchmarkpassword"
	params := DefaultArgon2idParams()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashPassword(password, params)
	}
}

func BenchmarkVerifyPassword(b *testing.B) {
	password := "benchmarkpassword"
	hash, _ := HashPassword(password, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyPassword(password, hash)
	}
}
