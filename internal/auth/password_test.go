package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashPasswordThenVerify(t *testing.T) {
	hash := HashPassword("correct horse battery staple")

	testCases := []struct {
		name     string
		password string
		want     bool
	}{
		{name: "the right password", password: "correct horse battery staple", want: true},
		{name: "a wrong password", password: "correct horse battery stapler", want: false},
		{name: "the right password in another case", password: "Correct Horse Battery Staple", want: false},
		{name: "an empty password", password: "", want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := VerifyPassword(testCase.password, hash)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != testCase.want {
				t.Errorf("VerifyPassword = %t, want %t", got, testCase.want)
			}
		})
	}
}

func TestHashPasswordFormat(t *testing.T) {
	hash := HashPassword("correct horse battery staple")

	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("hash = %q, want the argon2id PHC format with the OWASP minimum settings", hash)
	}
	if strings.Contains(hash, "correct") {
		t.Error("hash contains the password")
	}
	// 16 bytes of salt and 32 bytes of key, in base64 without padding.
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || len(parts[4]) != 22 || len(parts[5]) != 43 {
		t.Errorf("hash = %q, want a 22 character salt and a 43 character key", hash)
	}
}

func TestHashPasswordUsesANewSaltEveryTime(t *testing.T) {
	first := HashPassword("correct horse battery staple")
	second := HashPassword("correct horse battery staple")

	if first == second {
		t.Error("two hashes of the same password are identical, so the salt is not random")
	}
}

func TestVerifyPasswordUsesTheSettingsInTheHash(t *testing.T) {
	// Made with weaker settings than HashPassword uses now (m=8192, t=1), as an older hash would be.
	const olderHash = "$argon2id$v=19$m=8192,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$xwP/sUpncRUpT63+0aadhrYrW8PTzYL8TrQp/6Tj9lA"

	matches, err := VerifyPassword("correct horse battery staple", olderHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matches {
		t.Error("a hash made with older settings no longer verifies")
	}
}

func TestVerifyPasswordRejectsHashesItCannotTrust(t *testing.T) {
	const salt = "c2FsdHNhbHRzYWx0c2FsdA"
	const key = "xwP/sUpncRUpT63+0aadhrYrW8PTzYL8TrQp/6Tj9lA"

	testCases := []struct {
		name   string
		stored string
	}{
		{name: "empty", stored: ""},
		{name: "plain text", stored: "correct horse battery staple"},
		{name: "another algorithm", stored: "$argon2i$v=19$m=19456,t=2,p=1$" + salt + "$" + key},
		{name: "a bcrypt hash", stored: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"},
		{name: "another version", stored: "$argon2id$v=16$m=19456,t=2,p=1$" + salt + "$" + key},
		{name: "settings missing", stored: "$argon2id$v=19$" + salt + "$" + key},
		{name: "settings not numbers", stored: "$argon2id$v=19$m=lots,t=2,p=1$" + salt + "$" + key},
		{name: "zero passes", stored: "$argon2id$v=19$m=19456,t=0,p=1$" + salt + "$" + key},
		{name: "zero threads", stored: "$argon2id$v=19$m=19456,t=2,p=0$" + salt + "$" + key},
		{name: "four terabytes of memory", stored: "$argon2id$v=19$m=4294967295,t=2,p=1$" + salt + "$" + key},
		{name: "four billion passes", stored: "$argon2id$v=19$m=19456,t=4294967295,p=1$" + salt + "$" + key},
		{name: "salt not base64", stored: "$argon2id$v=19$m=19456,t=2,p=1$!!!$" + key},
		{name: "empty salt", stored: "$argon2id$v=19$m=19456,t=2,p=1$$" + key},
		{name: "key not base64", stored: "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$!!!"},
		{name: "empty key", stored: "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$"},
		{name: "key of the wrong length", stored: "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$c2hvcnQ"},
		{name: "text before the hash", stored: "x$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$" + key},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			matches, err := VerifyPassword("correct horse battery staple", testCase.stored)
			if !errors.Is(err, ErrInvalidHash) {
				t.Errorf("error = %v, want ErrInvalidHash", err)
			}
			if matches {
				t.Error("a hash that cannot be trusted was accepted")
			}
		})
	}
}
