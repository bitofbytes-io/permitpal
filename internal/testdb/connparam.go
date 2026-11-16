package testdb

import (
	"fmt"
	"net/url"
	"strings"
)

// SetConnParam returns conn with the connection parameter key set to value and
// every other parameter preserved. conn may be a postgres:// or postgresql://
// URL or a keyword/value DSN, the two forms pgx accepts. An existing key is
// replaced, not duplicated, so the result is unambiguous for pgx.
func SetConnParam(conn, key, value string) (string, error) {
	if strings.HasPrefix(conn, "postgres://") || strings.HasPrefix(conn, "postgresql://") {
		u, err := url.Parse(conn)
		if err != nil {
			return "", fmt.Errorf("parse database URL: %w", err)
		}
		query := u.Query()
		query.Set(key, value)
		u.RawQuery = query.Encode()
		return u.String(), nil
	}
	fields, err := splitKeywordValue(conn)
	if err != nil {
		return "", err
	}
	kept := make([]string, 0, len(fields)+1)
	for _, field := range fields {
		if field.key != key {
			kept = append(kept, field.raw)
		}
	}
	quoted := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value)
	return strings.Join(append(kept, key+"='"+quoted+"'"), " "), nil
}

type dsnField struct{ key, raw string }

// splitKeywordValue splits a keyword/value DSN into its fields, honouring
// single-quoted values and backslash escapes. Spaces around "=" are allowed.
func splitKeywordValue(dsn string) ([]dsnField, error) {
	var fields []dsnField
	for rest := strings.TrimSpace(dsn); rest != ""; rest = strings.TrimSpace(rest) {
		eq := strings.IndexByte(rest, '=')
		if eq < 0 {
			return nil, fmt.Errorf("invalid keyword/value connection string: missing %q", "=")
		}
		key := strings.TrimSpace(rest[:eq])
		if key == "" || strings.ContainsAny(key, " \t\n") {
			return nil, fmt.Errorf("invalid keyword/value connection string: bad keyword %q", key)
		}
		i := eq + 1
		for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t') {
			i++
		}
		if i < len(rest) && rest[i] == '\'' {
			i++
			for i < len(rest) && rest[i] != '\'' {
				if rest[i] == '\\' {
					i++
				}
				i++
			}
			if i >= len(rest) {
				return nil, fmt.Errorf("invalid keyword/value connection string: unterminated quote for %q", key)
			}
			i++
		} else {
			for i < len(rest) && rest[i] != ' ' && rest[i] != '\t' && rest[i] != '\n' {
				if rest[i] == '\\' {
					i++
				}
				i++
			}
			if i > len(rest) {
				i = len(rest)
			}
		}
		fields = append(fields, dsnField{key: key, raw: rest[:i]})
		rest = rest[i:]
	}
	return fields, nil
}
