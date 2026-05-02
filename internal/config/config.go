package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	DataStoreMemory   = "memory"
	DataStorePostgres = "postgres"
)

type Config struct {
	AppEnv          string
	DataStore       string
	Port            string
	DatabaseURL     string
	Password        string
	PasswordHash    string
	SessionSecret   string
	SecureCookies   bool
	SessionCookie   string
	DefaultUsername string
}

func Load() (*Config, error) {
	cfg := &Config{}
	var err error

	cfg.AppEnv = getEnv("APP_ENV", "development")
	cfg.DataStore = strings.ToLower(getEnv("DATA_STORE", defaultDataStore(cfg.AppEnv)))
	cfg.Port = getEnv("PORT", "4600")
	cfg.DatabaseURL, err = getEnvOrFile("DATABASE_URL", "/run/secrets/permitpal_database_url")
	if err != nil {
		return nil, err
	}
	cfg.Password, err = getEnvOrFile("PERMITPAL_PASSWORD", "")
	if err != nil {
		return nil, err
	}
	cfg.PasswordHash, err = getEnvOrFile("PERMITPAL_PASSWORD_HASH", "/run/secrets/permitpal_password_hash")
	if err != nil {
		return nil, err
	}
	cfg.SessionSecret, err = getEnvOrFile("SESSION_SECRET", "/run/secrets/permitpal_session_secret")
	if err != nil {
		return nil, err
	}
	cfg.SessionCookie = getEnv("SESSION_COOKIE", "permitpal_session")
	cfg.DefaultUsername = getEnv("PERMITPAL_USERNAME", "driver")
	cfg.SecureCookies = getEnv("SECURE_COOKIES", defaultSecureCookies(cfg.AppEnv)) != "false"

	if cfg.DataStore != DataStoreMemory && cfg.DataStore != DataStorePostgres {
		return nil, fmt.Errorf("DATA_STORE must be memory or postgres, got %q", cfg.DataStore)
	}
	if cfg.DataStore == DataStorePostgres && cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required when DATA_STORE=postgres")
	}

	if cfg.AppEnv == "production" {
		if cfg.PasswordHash == "" {
			return nil, errors.New("PERMITPAL_PASSWORD_HASH is required in production")
		}
		if cfg.SessionSecret == "" {
			return nil, errors.New("SESSION_SECRET is required in production")
		}
	}

	if cfg.Password == "" && cfg.PasswordHash == "" {
		cfg.Password = "permitpal"
	}
	if cfg.SessionSecret == "" {
		cfg.SessionSecret = "permitpal-local-dev-session-secret-change-me"
	}

	return cfg, nil
}

func defaultDataStore(appEnv string) string {
	if appEnv == "production" {
		return DataStorePostgres
	}
	return DataStoreMemory
}

func defaultSecureCookies(appEnv string) string {
	if appEnv == "production" {
		return "true"
	}
	return "false"
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvOrFile(key, defaultPath string) (string, error) {
	if value := os.Getenv(key); value != "" {
		return strings.TrimSpace(value), nil
	}
	if path := os.Getenv(key + "_FILE"); path != "" {
		return readSecret(path, key+"_FILE")
	}
	if defaultPath != "" {
		return readSecret(defaultPath, key)
	}
	return "", nil
}

func readSecret(path, name string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("reading %s from %s: %w", name, path, err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("%s at %s is empty", name, path)
	}
	return value, nil
}
