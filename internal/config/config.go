package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Environment string
	ListenAddr string
}

var validEnvronments = map[string]bool {
	"development": true,
	"production": true,
}

func Load() (Config, error) {
	cfg := Config{
		Environment: envOrDefualt("APP_ENV", "development"),
		ListenAddr: envOrDefualt("LISTEN_ADDR", ":8080"),
	}

	var problems []string
	if !validEnvronments[cfg.Environment] {
		problems = append(problems, fmt.Sprintf("APP_ENV must be development or production, got %q", cfg.Environment))
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %s", strings.Join(problems, "; "))
	}
	return cfg, nil
}

func envOrDefualt(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return  value
	}
	return fallback
}
