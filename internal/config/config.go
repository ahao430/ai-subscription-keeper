package config

import (
	"os"
)

// Config holds process level configuration, sourced from environment variables.
type Config struct {
	Port          string
	DataDir       string
	EncryptionKey string
}

func Load() Config {
	return Config{
		Port:          envOr("PORT", "8080"),
		DataDir:       envOr("DATA_DIR", "./data"),
		EncryptionKey: envOr("WARMUP_ENCRYPTION_KEY", ""),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
