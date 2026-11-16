package testdb

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestSetConnParamReplacesApplicationNameAndKeepsTheRest(t *testing.T) {
	for _, tc := range []struct {
		name, conn string
	}{
		{"url with application_name", "postgres://u:p%40ss@db.example:5433/app?sslmode=disable&application_name=ci_runner&connect_timeout=7"},
		{"url without query", "postgresql://u:p@db.example/app"},
		{"keyword/value with application_name", "host=db.example port=5433 user=u password='p ss' dbname=app sslmode=disable application_name=ci_runner connect_timeout=7"},
		{"keyword/value spaced and quoted", "host = db.example user=u password='it\\'s' dbname=app application_name = 'ci runner'"},
		{"keyword/value without application_name", "host=db.example user=u dbname=app"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := pgconn.ParseConfig(tc.conn)
			if err != nil {
				t.Fatal(err)
			}
			got, err := SetConnParam(tc.conn, "application_name", "child_probe")
			if err != nil {
				t.Fatal(err)
			}
			after, err := pgconn.ParseConfig(got)
			if err != nil {
				t.Fatalf("rewritten connection string does not parse: %v", err)
			}
			if after.RuntimeParams["application_name"] != "child_probe" {
				t.Fatalf("application_name = %q, want child_probe", after.RuntimeParams["application_name"])
			}
			if after.Host != before.Host || after.Port != before.Port || after.User != before.User ||
				after.Password != before.Password || after.Database != before.Database ||
				after.ConnectTimeout != before.ConnectTimeout {
				t.Fatalf("rewrite changed other settings: before %+v, after %+v", before, after)
			}
			for key, value := range before.RuntimeParams {
				if key != "application_name" && after.RuntimeParams[key] != value {
					t.Fatalf("runtime param %s = %q, want %q", key, after.RuntimeParams[key], value)
				}
			}
		})
	}
}

func TestSetConnParamIgnoresPGAPPNAME(t *testing.T) {
	t.Setenv("PGAPPNAME", "from_environment")
	got, err := SetConnParam("postgres://u@db.example/app?application_name=from_url", "application_name", "explicit")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pgconn.ParseConfig(got)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RuntimeParams["application_name"] != "explicit" {
		t.Fatalf("application_name = %q, want explicit", cfg.RuntimeParams["application_name"])
	}
}

func TestSetConnParamRejectsMalformedKeywordValue(t *testing.T) {
	for _, conn := range []string{"host", "host=db password='unterminated"} {
		if _, err := SetConnParam(conn, "application_name", "x"); err == nil {
			t.Fatalf("SetConnParam(%q) succeeded, want an error", conn)
		}
	}
}
