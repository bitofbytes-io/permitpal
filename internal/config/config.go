package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // The Alpine image has no zoneinfo; embed it for APP_TIMEZONE.
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	AppEnvDevelopment      = "development"
	AppEnvProduction       = "production"
	DataStoreMemory        = "memory"
	DataStorePostgres      = "postgres"
	minSessionSecretLength = 32
)

type Config struct {
	Users         map[string]string
	AppEnv        string
	DataStore     string
	LogLevel      string
	Port          string
	DatabaseURL   string
	SessionSecret string
	SecureCookies bool
	SessionCookie string
	// TrustedProxies lists peers whose X-Forwarded-For header is honored.
	TrustedProxies []netip.Prefix
	// Location decides the calendar date for "today" and submitted dates.
	Location *time.Location
}

func Load() (*Config, error) {
	cfg := &Config{}
	var err error

	cfg.AppEnv = strings.ToLower(strings.TrimSpace(getEnv("APP_ENV", AppEnvDevelopment)))
	// Production turns on the protections, so a typo must not silently fall back to development.
	if cfg.AppEnv != AppEnvDevelopment && cfg.AppEnv != AppEnvProduction {
		return nil, fmt.Errorf("APP_ENV must be development or production, got %q", cfg.AppEnv)
	}
	cfg.DataStore = strings.ToLower(getEnv("DATA_STORE", defaultDataStore(cfg.AppEnv)))
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(getEnv("LOG_LEVEL", "info")))
	cfg.Port = getEnv("PORT", "4600")
	cfg.DatabaseURL, err = getEnvOrFile("DATABASE_URL", "/run/secrets/permitpal_database_url")
	if err != nil {
		return nil, err
	}
	cfg.SessionSecret, err = getEnvOrFile("SESSION_SECRET", "/run/secrets/permitpal_session_secret")
	if err != nil {
		return nil, err
	}
	cfg.SessionCookie = getEnv("SESSION_COOKIE", "permitpal_session")
	users, err := getEnvOrFile("PERMITPAL_USERS", "/run/secrets/permitpal_users")
	if err != nil {
		return nil, err
	}
	cfg.Users, err = parseUsers(users)
	if err != nil {
		return nil, err
	}
	cfg.SecureCookies, err = parseBoolEnv("SECURE_COOKIES", defaultSecureCookies(cfg.AppEnv))
	if err != nil {
		return nil, err
	}

	timezone := strings.TrimSpace(getEnv("APP_TIMEZONE", "America/New_York"))
	cfg.Location, err = time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("APP_TIMEZONE must be an IANA time zone, got %q: %w", timezone, err)
	}
	cfg.TrustedProxies, err = parseTrustedProxies(os.Getenv("TRUSTED_PROXY_CIDRS"))
	if err != nil {
		return nil, err
	}

	if cfg.DataStore != DataStoreMemory && cfg.DataStore != DataStorePostgres {
		return nil, fmt.Errorf("DATA_STORE must be memory or postgres, got %q", cfg.DataStore)
	}
	if cfg.DataStore == DataStorePostgres && cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required when DATA_STORE=postgres")
	}

	if cfg.AppEnv == AppEnvProduction && cfg.SessionSecret == "" {
		return nil, errors.New("SESSION_SECRET is required in production")
	}

	if len(cfg.Users) == 0 {
		return nil, errors.New("PERMITPAL_USERS is required")
	}
	if cfg.SessionSecret == "" {
		return nil, errors.New("SESSION_SECRET is required")
	}
	if utf8.RuneCountInString(cfg.SessionSecret) < minSessionSecretLength {
		return nil, fmt.Errorf("SESSION_SECRET must be at least %d characters", minSessionSecretLength)
	}

	return cfg, nil
}

func defaultDataStore(appEnv string) string {
	if appEnv == AppEnvProduction {
		return DataStorePostgres
	}
	return DataStoreMemory
}

func defaultSecureCookies(appEnv string) string {
	if appEnv == AppEnvProduction {
		return "true"
	}
	return "false"
}

// parseTrustedProxies reads a comma-separated list of CIDRs or single IPs.
func parseTrustedProxies(value string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			// Client addresses are unmapped before matching, so rebase
			// IPv4-mapped IPv6 prefixes onto IPv4.
			if prefix.Addr().Is4In6() {
				if prefix.Bits() < 96 {
					return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS IPv4-mapped prefix %q must be /96 or longer", entry)
				}
				prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
			}
			prefixes = append(prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS contains invalid CIDR or IP %q", entry)
		}
		addr = addr.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return prefixes, nil
}

func parseBoolEnv(key, fallback string) (bool, error) {
	value := getEnv(key, fallback)
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean value, got %q", key, value)
	}
	return parsed, nil
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
	if path, explicit := os.LookupEnv(key + "_FILE"); explicit {
		if path == "" {
			return "", nil
		}
		return readSecret(path, key+"_FILE", false)
	}
	if defaultPath != "" {
		return readSecret(defaultPath, key, true)
	}
	return "", nil
}

func readSecret(path, name string, allowMissing bool) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if allowMissing && errors.Is(err, os.ErrNotExist) {
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

var validUsername = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ValidUsername reports whether username matches the account username rule.
func ValidUsername(username string) bool { return validUsername.MatchString(username) }

var bcryptEncoding = regexp.MustCompile(`^\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}$`)

func validBcrypt(hash string) bool {
	if !bcryptEncoding.MatchString(hash) {
		return false
	}
	_, err := bcrypt.Cost([]byte(hash))
	return err == nil
}
func parseUsers(value string) (map[string]string, error) {
	users := make(map[string]string)
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		username, hash, ok := strings.Cut(line, ":")
		if !ok || !validUsername.MatchString(username) {
			return nil, errors.New("PERMITPAL_USERS contains an invalid username")
		}
		if _, exists := users[username]; exists {
			return nil, fmt.Errorf("PERMITPAL_USERS contains duplicate username %q", username)
		}
		if !validBcrypt(hash) {
			return nil, fmt.Errorf("PERMITPAL_USERS credential for %q must be a bcrypt hash", username)
		}
		users[username] = hash
	}
	return users, nil
}
