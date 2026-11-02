// Package config loads process configuration from environment variables.
package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL        string
	MigrateDatabaseURL string
	HTTPAddr           string
	StorageDir         string
	JWTSecret          []byte
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		MigrateDatabaseURL: os.Getenv("MIGRATE_DATABASE_URL"),
		HTTPAddr:           getenv("HTTP_ADDR", ":8080"),
		StorageDir:         getenv("STORAGE_DIR", "./data"),
		JWTSecret:          []byte(os.Getenv("JWT_SECRET")),
	}
	if c.DatabaseURL == "" {
		return Config{}, errors.New("config: DATABASE_URL is required")
	}
	if len(c.JWTSecret) < 32 {
		return Config{}, errors.New("config: JWT_SECRET must be at least 32 bytes")
	}
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
