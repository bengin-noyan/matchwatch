package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func clearEnv(t *testing.T) {
	for _, k := range []string{"DATABASE_URL", "MIGRATE_DATABASE_URL", "HTTP_ADDR", "STORAGE_DIR", "JWT_SECRET", "AI_SERVICE_URL", "AI_SERVICE_TOKEN", "AI_EXTRACT_TIMEOUT"} {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", strings.Repeat("a", 32))
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":8080" || c.StorageDir != "./data" {
		t.Fatalf("defaults: %+v", c)
	}
}

func TestLoadRejectsShortSecret(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", strings.Repeat("a", 31))
	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("JWT_SECRET", strings.Repeat("a", 32))
	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}

func baseEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", strings.Repeat("a", 32))
}

func TestAIDefaultsNoTokenNeeded(t *testing.T) {
	baseEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.AIServiceURL != "" || c.AIExtractTimeout != 5*time.Minute {
		t.Fatalf("%+v", c)
	}
}

func TestAIShortTokenRejected(t *testing.T) {
	baseEnv(t)
	t.Setenv("AI_SERVICE_URL", "http://ai:8000")
	t.Setenv("AI_SERVICE_TOKEN", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
	t.Setenv("AI_SERVICE_TOKEN", strings.Repeat("t", 32))
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}

func TestAIBadTimeoutRejected(t *testing.T) {
	for _, v := range []string{"abc", "0", "-1s"} {
		baseEnv(t)
		t.Setenv("AI_EXTRACT_TIMEOUT", v)
		if _, err := Load(); err == nil {
			t.Fatalf("expected error for %q", v)
		}
	}
}

func TestTCMBBaseURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", strings.Repeat("a", 32))
	t.Setenv("TCMB_BASE_URL", "")
	os.Unsetenv("TCMB_BASE_URL") // t.Setenv restores it afterwards
	if c, err := Load(); err != nil || c.TCMBBaseURL != "https://www.tcmb.gov.tr/kurlar" {
		t.Fatalf("unset: %q %v", c.TCMBBaseURL, err)
	}
	t.Setenv("TCMB_BASE_URL", "")
	if c, err := Load(); err != nil || c.TCMBBaseURL != "" {
		t.Fatalf("empty must disable: %q %v", c.TCMBBaseURL, err)
	}
}
