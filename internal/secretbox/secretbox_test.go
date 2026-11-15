package secretbox

import (
	"bytes"
	"testing"
)

// nonce (12 bytes) plus authentication tag (16 bytes)
const overhead = 28

// Test keys differ only in their last byte, so a test fails if only part of the key is used.
func newTestBox(t *testing.T, lastKeyByte byte) *Box {
	t.Helper()
	key := bytes.Repeat([]byte{1}, KeySize)
	key[KeySize-1] = lastKeyByte
	box, err := New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return box
}

func TestOpenReturnsWhatWasSealed(t *testing.T) {
	box := newTestBox(t, 1)
	testCases := map[string][]byte{
		"a signing secret": []byte("a demo signing secret"),
		"nothing":          {},
		"one zero byte":    {0},
		"a long value":     bytes.Repeat([]byte("x"), 4096),
	}

	for name, plaintext := range testCases {
		t.Run(name, func(t *testing.T) {
			sealed := box.Seal(plaintext)

			opened, err := box.Open(sealed)

			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if !bytes.Equal(opened, plaintext) {
				t.Errorf("opened %q, want %q", opened, plaintext)
			}
		})
	}
}

func TestSealHidesThePlaintext(t *testing.T) {
	box := newTestBox(t, 1)
	plaintext := []byte("a demo signing secret")

	sealed := box.Seal(plaintext)

	if bytes.Contains(sealed, plaintext) {
		t.Error("the sealed value contains the plaintext")
	}
	if len(sealed) != len(plaintext)+overhead {
		t.Errorf("sealed value is %d bytes, want %d", len(sealed), len(plaintext)+overhead)
	}
}

func TestSealGivesADifferentResultEachTime(t *testing.T) {
	box := newTestBox(t, 1)
	plaintext := []byte("a demo signing secret")

	first := box.Seal(plaintext)
	second := box.Seal(plaintext)

	if bytes.Equal(first, second) {
		t.Error("sealing the same plaintext twice gave the same result")
	}
}

func TestOpenRejectsAnAlteredValue(t *testing.T) {
	box := newTestBox(t, 1)
	sealed := box.Seal([]byte("a demo signing secret"))

	// Covers the nonce, the ciphertext and the authentication tag.
	for position := range sealed {
		altered := bytes.Clone(sealed)
		altered[position] ^= 1

		if _, err := box.Open(altered); err == nil {
			t.Errorf("opened a value with byte %d altered", position)
		}
	}
}

func TestOpenRejectsAValueCutShort(t *testing.T) {
	box := newTestBox(t, 1)
	sealed := box.Seal([]byte("a demo signing secret"))

	for length := range sealed {
		if _, err := box.Open(sealed[:length]); err == nil {
			t.Errorf("opened a value cut down to %d bytes", length)
		}
	}
}

func TestOpenRejectsAValueSealedWithAnotherKey(t *testing.T) {
	sealed := newTestBox(t, 1).Seal([]byte("a demo signing secret"))

	if _, err := newTestBox(t, 2).Open(sealed); err == nil {
		t.Error("opened a value sealed with another key")
	}
}

func TestNewRejectsAKeyThatIsNot32Bytes(t *testing.T) {
	// 16 and 24 bytes are valid AES keys, but not AES-256.
	for _, length := range []int{0, 16, 24, 31, 33, 64} {
		if _, err := New(make([]byte, length)); err == nil {
			t.Errorf("accepted a %d byte key", length)
		}
	}
}
