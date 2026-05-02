package config

import (
	"strings"
	"testing"
)

func TestLoadParsesSecureCookiesCaseInsensitively(t *testing.T) {
	t.Setenv("SECURE_COOKIES", "FALSE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.SecureCookies {
		t.Fatal("SecureCookies = true, want false")
	}
}

func TestLoadRejectsInvalidSecureCookies(t *testing.T) {
	t.Setenv("SECURE_COOKIES", "sometimes")

	if _, err := Load(); err == nil {
		t.Fatal("Load returned nil error")
	}
}

func TestLoadNormalizesAppEnv(t *testing.T) {
	t.Setenv("APP_ENV", "Production")
	t.Setenv("DATABASE_URL", "postgres://permitpal:permitpal@localhost:5432/permitpal?sslmode=disable")
	t.Setenv("PERMITPAL_PASSWORD_HASH", "hash")
	t.Setenv("SESSION_SECRET", "secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.AppEnv != "production" {
		t.Fatalf("AppEnv = %q, want production", cfg.AppEnv)
	}
	if cfg.DataStore != DataStorePostgres {
		t.Fatalf("DataStore = %q, want postgres", cfg.DataStore)
	}
	if !cfg.SecureCookies {
		t.Fatal("SecureCookies = false, want true")
	}
}

func TestLoadFailsForMissingExplicitSecretFile(t *testing.T) {
	t.Setenv("SESSION_SECRET_FILE", "/no/such/permitpal/session-secret")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SESSION_SECRET_FILE") {
		t.Fatalf("Load error = %v, want SESSION_SECRET_FILE error", err)
	}
}
