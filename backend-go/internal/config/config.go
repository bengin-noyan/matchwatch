// Package config loads process configuration from environment variables.
package config

import (
	"errors"
	"os"
	"strings"
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
	TCMBBaseURL        string // set but empty = TCMB rates disabled (D17)
	CookieSecure       bool   // session cookie only over HTTPS; COOKIE_SECURE=false for local http
	WebDir             string // built SPA (web/dist) served on the same origin; empty = API only
	TrustProxy         bool   // TRUST_PROXY=true: client IP from X-Forwarded-For (only when reachable solely via Caddy)
	MailURL            string // "" = no invites or "Şifremi unuttum"; "log" = dev; smtp://user:pass@host:587
	MailFrom           string
	AppBaseURL         string // the links in mails; never taken from the request's Host header
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
	if err != nil || d <= 0 {
		return Config{}, errors.New("config: AI_EXTRACT_TIMEOUT must be a positive duration")
	}
	c.AIExtractTimeout = d
	c.CookieSecure = os.Getenv("COOKIE_SECURE") != "false"
	c.WebDir = os.Getenv("WEB_DIR")
	c.TrustProxy = os.Getenv("TRUST_PROXY") == "true"
	c.MailURL, c.MailFrom = os.Getenv("MAIL_URL"), os.Getenv("MAIL_FROM")
	c.AppBaseURL = strings.TrimRight(os.Getenv("APP_BASE_URL"), "/")
	if c.MailURL != "" && !strings.HasPrefix(c.AppBaseURL, "http") {
		return Config{}, errors.New("config: APP_BASE_URL (e.g. https://matchwatch.example.com.tr) is required when MAIL_URL is set")
	}
	c.TCMBBaseURL = "https://www.tcmb.gov.tr/kurlar"
	if v, ok := os.LookupEnv("TCMB_BASE_URL"); ok {
		c.TCMBBaseURL = v
	}
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
