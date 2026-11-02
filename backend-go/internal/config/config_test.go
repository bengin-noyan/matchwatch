package config

import (
	"strings"
	"testing"
)

func clear(t *testing.T) {
	for _, k := range []string{"DATABASE_URL", "MIGRATE_DATABASE_URL", "HTTP_ADDR", "STORAGE_DIR", "JWT_SECRET"} {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clear(t)
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
	clear(t)
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", strings.Repeat("a", 31))
	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	clear(t)
	t.Setenv("JWT_SECRET", strings.Repeat("a", 32))
	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}
