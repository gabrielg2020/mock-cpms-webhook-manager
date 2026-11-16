// Package webhook holds the rules for webhooks that need neither the web nor the database.
package webhook

import (
	"crypto/rand"
	"encoding/base64"
)

// HMAC-SHA256 wants a key at least as long as its output
const secretBytes = 32

const shownCharacters = 4

// NewSecret returns a new signing secret as text, and its last four characters
// which are safe to show so that its owner can tell which secret is in use.
func NewSecret() (secret, lastFour string) {
	raw := make([]byte, secretBytes)
	rand.Read(raw)
	secret = base64.RawURLEncoding.EncodeToString(raw)
	return secret, secret[len(secret)-shownCharacters:]
}
