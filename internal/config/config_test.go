package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearAuthEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"PERMITPAL_PASSWORD", "PERMITPAL_PASSWORD_FILE",
		"PERMITPAL_USERS", "PERMITPAL_USERS_FILE", "PERMITPAL_USERNAME",
		"PERMITPAL_PASSWORD_HASH", "PERMITPAL_PASSWORD_HASH_FILE",
		"SESSION_SECRET", "SESSION_SECRET_FILE",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadParsesSecureCookiesCaseInsensitively(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("SECURE_COOKIES", "FALSE")
	t.Setenv("PERMITPAL_PASSWORD_HASH", testHash)
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
	t.Setenv("PERMITPAL_PASSWORD_HASH", testHash)
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

func TestLoadNormalizesLogLevel(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("LOG_LEVEL", "WARN")
	t.Setenv("PERMITPAL_PASSWORD_HASH", testHash)
	t.Setenv("SESSION_SECRET", "local-session-secret-32-bytes-ok")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.LogLevel != "warn" {
		t.Fatalf("LogLevel = %q, want warn", cfg.LogLevel)
	}
}

func TestLoadFailsForMissingExplicitSecretFile(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("PERMITPAL_PASSWORD_HASH", testHash)
	t.Setenv("SESSION_SECRET_FILE", "/no/such/permitpal/session-secret")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SESSION_SECRET_FILE") {
		t.Fatalf("Load error = %v, want SESSION_SECRET_FILE error", err)
	}
}

func TestLoadRequiresPasswordSecretInDevelopment(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("APP_ENV", "development")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "PERMITPAL_USERS, PERMITPAL_PASSWORD_HASH or PERMITPAL_PASSWORD is required") {
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

func TestLoadRejectsMultibyteSessionSecretWithTooFewCharacters(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("PERMITPAL_PASSWORD", "local-password")
	t.Setenv("SESSION_SECRET", strings.Repeat("😀", 8))

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SESSION_SECRET must be at least 32 characters") {
		t.Fatalf("Load error = %v, want short session secret error", err)
	}
}

func TestLoadRejectsMissingSecretsOutsideDevelopment(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("APP_ENV", "staging")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "PERMITPAL_USERS, PERMITPAL_PASSWORD_HASH or PERMITPAL_PASSWORD is required") {
		t.Fatalf("Load error = %v, want missing password secret error", err)
	}
}

const testHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func TestUsersFileAndProductionRules(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATA_STORE", "memory")
	t.Setenv("SESSION_SECRET", strings.Repeat("s", 32))
	path := filepath.Join(t.TempDir(), "users")
	if err := os.WriteFile(path, []byte("caleb:"+testHash+"\naiden:"+strings.Replace(testHash, "$2a$", "$2y$", 1)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PERMITPAL_USERS_FILE", path)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Users) != 2 || !cfg.SecureCookies {
		t.Fatalf("bad config: %+v", cfg)
	}
	t.Setenv("PERMITPAL_PASSWORD", "plain")
	if _, err := Load(); err == nil {
		t.Fatal("production accepted plaintext")
	}
}
func TestUsersValidation(t *testing.T) {
	for _, value := range []string{"Aiden:" + testHash, "a:b:" + testHash, "-aiden:" + testHash, strings.Repeat("a", 33) + ":" + testHash, "aiden:plaintext", "aiden:$2a$10$bad", "aiden:" + testHash + "\naiden:" + testHash} {
		if _, err := parseUsers(value); err == nil {
			t.Errorf("accepted invalid users: %q", value)
		}
	}
	clearAuthEnv(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("PERMITPAL_USERS", "driver:"+testHash)
	t.Setenv("PERMITPAL_PASSWORD_HASH", testHash)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate legacy error: %v", err)
	}
}
