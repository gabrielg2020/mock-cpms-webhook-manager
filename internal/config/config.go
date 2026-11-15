package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/secretbox"
)

// A seeded password is the only thing between the public site and the demo accounts.
const minSeedPasswordLength = 12

type Config struct {
	Environment         string
	ListenAddr          string
	DatabasePath        string
	SeedDemoData        bool
	SeedAdminPassword   string
	SeedMemberPassword  string
	SecureCookies       bool
	SecretEncryptionKey []byte
}

var validEnvironments = map[string]bool{
	"development": true,
	"production":  true,
}

func Load() (Config, error) {
	cfg := Config{
		Environment:        envOrDefault("APP_ENV", "development"),
		ListenAddr:         envOrDefault("LISTEN_ADDR", ":8080"),
		DatabasePath:       envOrDefault("DATABASE_PATH", "data/app.db"),
		SeedAdminPassword:  os.Getenv("SEED_ADMIN_PASSWORD"),
		SeedMemberPassword: os.Getenv("SEED_MEMBER_PASSWORD"),
	}

	var problems []string
	if !validEnvironments[cfg.Environment] {
		problems = append(problems, fmt.Sprintf("APP_ENV must be development or production, got %q", cfg.Environment))
	}

	switch seed := envOrDefault("SEED_DEMO_DATA", "false"); seed {
	case "true":
		cfg.SeedDemoData = true
	case "false":
	default:
		problems = append(problems, fmt.Sprintf("SEED_DEMO_DATA must be true or false, got %q", seed))
	}
	if cfg.SeedDemoData {
		// The messages name the variable and never include its value.
		if len(cfg.SeedAdminPassword) < minSeedPasswordLength {
			problems = append(problems, fmt.Sprintf("SEED_ADMIN_PASSWORD must be at least %d characters when SEED_DEMO_DATA is true", minSeedPasswordLength))
		}
		if len(cfg.SeedMemberPassword) < minSeedPasswordLength {
			problems = append(problems, fmt.Sprintf("SEED_MEMBER_PASSWORD must be at least %d characters when SEED_DEMO_DATA is true", minSeedPasswordLength))
		}
	}

	// Cookies are sent of HTTPS only unless this is turned off, which only happens in development
	switch secure := envOrDefault("SECURE_COOKIES", "true"); secure {
	case "true":
		cfg.SecureCookies = true
	case "false":
		if cfg.Environment == "production" {
			problems = append(problems, "SECURE_COOKIES cannot be false when APP_ENV is production")
		}
	default:
		problems = append(problems, fmt.Sprintf("SECURE_COOKIES must be true or false, got %q", secure))
	}

	key, err := base64.StdEncoding.DecodeString(os.Getenv("SECRET_ENCRYPTION_KEY"))
	switch {
	case err != nil:
		problems = append(problems, "SECRET_ENCRYPTION_KEY must be base64")
	case len(key) != secretbox.KeySize:
		problems = append(problems, fmt.Sprintf("SECRET_ENCRYPTION_KEY must be %d bytes encoded as base64, got %d bytes", secretbox.KeySize, len(key)))
	default:
		cfg.SecretEncryptionKey = key
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %s", strings.Join(problems, "; "))
	}
	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
