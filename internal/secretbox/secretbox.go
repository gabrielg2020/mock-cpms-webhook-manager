package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
)

// KeySize is the length of an AES-256 key in bytes.
const KeySize = 32

type Box struct {
	aead cipher.AEAD
}

func New(key []byte) (*Box, error) {
	// aes.NewCipher also accepts 16 and 24 byte keys, which would quietly give AES-128 or AES-192.
	if len(key) != KeySize {
		return nil, fmt.Errorf("secretbox: key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: create cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: create GCM: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext. The results starts with a new random nonce, so
// sealing the same plaintext twice gives two different results.
func (box *Box) Seal(plaintext []byte) []byte {
	//nolint:gosec // G407 flase positive: NewGCMWithRandomNonce makes the nonce
	// itself, so the nonce argument must be empty.
	return box.aead.Seal(nil, nil, plaintext, nil)
}

// Open decrypts a value made by Seal. It fails if the value has been altered,
// cut short or sealed with a different key.
func (box *Box) Open(sealed []byte) ([]byte, error) {
	plaintext, err := box.aead.Open(nil, nil, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("secretbox: open: %w", err)
	}
	return plaintext, nil
}
