package config

import "testing"

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
