package config

import (
	"strings"
	"testing"
)

func clearAuthEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"PERMITPAL_PASSWORD", "PERMITPAL_PASSWORD_FILE",
		"PERMITPAL_PASSWORD_HASH", "PERMITPAL_PASSWORD_HASH_FILE",
		"SESSION_SECRET", "SESSION_SECRET_FILE",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadParsesSecureCookiesCaseInsensitively(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("SECURE_COOKIES", "FALSE")
	t.Setenv("PERMITPAL_PASSWORD_HASH", "local-password-hash")
	t.Setenv("SESSION_SECRET", "local-session-secret-32-bytes-ok")

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
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/permitpal?sslmode=disable")
	t.Setenv("PERMITPAL_PASSWORD_HASH", "hash")
	t.Setenv("SESSION_SECRET", "production-session-secret-32-bytes-ok")

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

func TestLoadRequiresPasswordSecretInDevelopment(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("APP_ENV", "development")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "PERMITPAL_PASSWORD_HASH or PERMITPAL_PASSWORD is required") {
		t.Fatalf("Load error = %v, want missing password secret error", err)
	}
}

func TestLoadRequiresSessionSecretInDevelopment(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("PERMITPAL_PASSWORD", "local-password")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SESSION_SECRET is required") {
		t.Fatalf("Load error = %v, want missing session secret error", err)
	}
}

func TestLoadRejectsShortSessionSecret(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("PERMITPAL_PASSWORD", "local-password")
	t.Setenv("SESSION_SECRET", "short")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SESSION_SECRET must be at least 32 characters") {
		t.Fatalf("Load error = %v, want short session secret error", err)
	}
}

func TestLoadRejectsMissingSecretsOutsideDevelopment(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("APP_ENV", "staging")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "PERMITPAL_PASSWORD_HASH or PERMITPAL_PASSWORD is required") {
		t.Fatalf("Load error = %v, want missing password secret error", err)
	}
}
