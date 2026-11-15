package config

import (
	"bytes"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/secretbox"
)

// 0xfb encodes to "+" and "/", the two characters that differ between standard and URL-safe base64.
var testKey = bytes.Repeat([]byte{0xfb}, secretbox.KeySize)

func encoded(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}

func TestLoad(t *testing.T) {
	testCases := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr bool
	}{
		{
			name: "uses defaults when only the key is set",
			env:  map[string]string{},
			want: Config{Environment: "development", ListenAddr: ":8080", DatabasePath: "data/app.db", SecureCookies: true},
		},
		{
			name: "reads values from the environment",
			env:  map[string]string{"APP_ENV": "production", "LISTEN_ADDR": ":9000", "DATABASE_PATH": "/data/app.db"},
			want: Config{Environment: "production", ListenAddr: ":9000", DatabasePath: "/data/app.db", SecureCookies: true},
		},
		{
			name:    "rejects an unknown environment",
			env:     map[string]string{"APP_ENV": "staging"},
			wantErr: true,
		},
		{
			name: "reads the seed settings",
			env:  map[string]string{"SEED_DEMO_DATA": "true", "SEED_ADMIN_PASSWORD": "admin-password-1", "SEED_MEMBER_PASSWORD": "member-password-1"},
			want: Config{
				Environment: "development", ListenAddr: ":8080", DatabasePath: "data/app.db",
				SeedDemoData: true, SeedAdminPassword: "admin-password-1", SeedMemberPassword: "member-password-1",
				SecureCookies: true,
			},
		},
		{
			name: "does not need seed passwords when seeding is off",
			env:  map[string]string{"SEED_DEMO_DATA": "false"},
			want: Config{Environment: "development", ListenAddr: ":8080", DatabasePath: "data/app.db", SecureCookies: true},
		},
		{
			name:    "rejects a seed setting that is not true or false",
			env:     map[string]string{"SEED_DEMO_DATA": "yes"},
			wantErr: true,
		},
		{
			name:    "rejects seeding without passwords",
			env:     map[string]string{"SEED_DEMO_DATA": "true"},
			wantErr: true,
		},
		{
			name:    "rejects a short seed password",
			env:     map[string]string{"SEED_DEMO_DATA": "true", "SEED_ADMIN_PASSWORD": "short", "SEED_MEMBER_PASSWORD": "member-password-1"},
			wantErr: true,
		},
		{
			name: "lets development turn secure cookies off",
			env:  map[string]string{"SECURE_COOKIES": "false"},
			want: Config{Environment: "development", ListenAddr: ":8080", DatabasePath: "data/app.db"},
		},
		{
			name:    "refuses to turn secure cookies off in production",
			env:     map[string]string{"APP_ENV": "production", "SECURE_COOKIES": "false"},
			wantErr: true,
		},
		{
			name:    "rejects a secure cookies setting that is not true or false",
			env:     map[string]string{"SECURE_COOKIES": "no"},
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("APP_ENV", "")
			t.Setenv("LISTEN_ADDR", "")
			t.Setenv("DATABASE_PATH", "")
			t.Setenv("SEED_DEMO_DATA", "")
			t.Setenv("SEED_ADMIN_PASSWORD", "")
			t.Setenv("SEED_MEMBER_PASSWORD", "")
			t.Setenv("SECURE_COOKIES", "")
			t.Setenv("SECRET_ENCRYPTION_KEY", encoded(testKey))
			for key, value := range testCase.env {
				t.Setenv(key, value)
			}

			got, err := Load()

			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			testCase.want.SecretEncryptionKey = testKey
			if !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("got %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestLoadNeverPutsAPasswordInAnErrorMessage(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", encoded(testKey))
	t.Setenv("SEED_DEMO_DATA", "true")
	t.Setenv("SEED_ADMIN_PASSWORD", "hunter2")
	t.Setenv("SEED_MEMBER_PASSWORD", "letmein")

	_, err := Load()

	if err == nil {
		t.Fatal("expected an error for short passwords, got nil")
	}
	for _, password := range []string{"hunter2", "letmein"} {
		if strings.Contains(err.Error(), password) {
			t.Errorf("error message contains the password %q: %v", password, err)
		}
	}
}

func TestLoadRefusesAMissingOrMalformedSecretEncryptionKey(t *testing.T) {
	const (
		notBase64   = "SECRET_ENCRYPTION_KEY must be base64"
		wrongLength = "SECRET_ENCRYPTION_KEY must be 32 bytes"
	)
	testCases := map[string]struct {
		value string
		want  string
	}{
		"not set":                      {"", wrongLength},
		"not base64":                   {"this is not base64!", notBase64},
		"an AES-128 key":               {encoded(testKey[:16]), wrongLength},
		"an AES-192 key":               {encoded(testKey[:24]), wrongLength},
		"one byte short":               {encoded(testKey[:31]), wrongLength},
		"one byte long":                {encoded(append(bytes.Clone(testKey), 0)), wrongLength},
		"URL-safe base64":              {base64.URLEncoding.EncodeToString(testKey), notBase64},
		"base64 without padding":       {base64.RawStdEncoding.EncodeToString(testKey), notBase64},
		"a valid key followed by junk": {encoded(testKey) + "!", notBase64},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("SECRET_ENCRYPTION_KEY", testCase.value)

			_, err := Load()

			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("got %q, want it to say %q", err, testCase.want)
			}
			if testCase.value != "" && strings.Contains(err.Error(), testCase.value) {
				t.Errorf("error message contains the key: %v", err)
			}
		})
	}
}
