// Package config loads process configuration from environment variables.
package config

import (
	"errors"
	"os"
	"time"
)

type Config struct {
	DatabaseURL        string
	MigrateDatabaseURL string
	HTTPAddr           string
	StorageDir         string
	JWTSecret          []byte
	AIServiceURL       string // empty = extraction disabled
	AIServiceToken     string
	AIExtractTimeout   time.Duration
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
	c.AIServiceURL = os.Getenv("AI_SERVICE_URL")
	c.AIServiceToken = os.Getenv("AI_SERVICE_TOKEN")
	if c.AIServiceURL != "" && len(c.AIServiceToken) < 32 {
		return Config{}, errors.New("config: AI_SERVICE_TOKEN must be at least 32 bytes when AI_SERVICE_URL is set")
	}
	d, err := time.ParseDuration(getenv("AI_EXTRACT_TIMEOUT", "5m"))
	if err != nil {
		return Config{}, errors.New("config: AI_EXTRACT_TIMEOUT is not a valid duration")
	}
	c.AIExtractTimeout = d
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
