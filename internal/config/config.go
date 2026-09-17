package config

import (
	"os"
)

// Config holds process level configuration, sourced from environment variables.
type Config struct {
	Port          string
	DataDir       string
	EncryptionKey string
	// AuthUsername / AuthPassword 启用内置登录；Password 为空 = 不启用（本机/内网模式）。
	AuthUsername string
	AuthPassword string
}

func Load() Config {
	return Config{
		Port:          envOr("PORT", "8080"),
		DataDir:       envOr("DATA_DIR", "./data"),
		EncryptionKey: envOr("WARMUP_ENCRYPTION_KEY", ""),
		AuthUsername:  envOr("AUTH_USERNAME", "admin"),
		AuthPassword:  os.Getenv("AUTH_PASSWORD"),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
