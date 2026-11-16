package webhook

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestNewSecretIs32RandomBytesAsText(t *testing.T) {
	secret, lastFour := NewSecret()

	raw, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil {
		t.Fatalf("secret is not URL-safe base64 without padding: %v", err)
	}
	if len(raw) != 32 {
		t.Errorf("secret holds %d bytes, want 32", len(raw))
	}
	if len(lastFour) != 4 || !strings.HasSuffix(secret, lastFour) {
		t.Errorf("last four = %q, want the last four characters of the secret", lastFour)
	}
}

func TestNewSecretIsDifferentEachTime(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		secret, _ := NewSecret()
		if seen[secret] {
			t.Fatal("the same secret was made twice")
		}
		seen[secret] = true
	}
}
