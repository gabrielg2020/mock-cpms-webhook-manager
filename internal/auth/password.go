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

// The settings for new hashes are the Argon2id minimum in the OWASP Password Storage Cheat Sheet:
// 19 MiB of memory, 2 passes over it, 1 thread.
const (
	memoryKiB  = 19 * 1024
	iterations = 2
	threads    = 1
	saltLength = 16
	keyLength  = 32
)

// VerifyPassword refuses a stored hash that asks for more work than these. A corrupted or
// tampered hash could otherwise ask for enough memory or time to take the server down.
const (
	maxMemoryKiB  = 64 * 1024
	maxIterations = 10
	maxThreads    = 4
)

var ErrInvalidHash = errors.New("password hash is not a valid argon2id hash")

// HashPassword returns the hash in PHC string format, e.g.
// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>. The settings and the salt are stored
// next to the hash, so the settings can be raised later without breaking existing passwords.
func HashPassword(password string) string {
	salt := make([]byte, saltLength)
	// crypto/rand.Read never returns an error
	_, _ = rand.Read(salt)

	key := argon2.IDKey([]byte(password), salt, iterations, memoryKiB, threads, keyLength)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memoryKiB, iterations, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
}

// VerifyPassword reports whether the password matches the stored hash. It uses the settings
// recorded in the hash, so hashes made with older settings still verify.
func VerifyPassword(password, stored string) (bool, error) {
	// A PHC string starts with "$", so the first part is empty.
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, ErrInvalidHash
	}

	var memory, passes uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &parallelism); err != nil {
		return false, ErrInvalidHash
	}
	if memory < 1 || memory > maxMemoryKiB || passes < 1 || passes > maxIterations || parallelism < 1 || parallelism > maxThreads {
		return false, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return false, ErrInvalidHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) != keyLength {
		return false, ErrInvalidHash
	}

	got := argon2.IDKey([]byte(password), salt, passes, memory, parallelism, keyLength)

	// A constant-time comparison to stop timing attacks
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
