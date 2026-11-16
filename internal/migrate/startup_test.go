package migrate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/testdb"
	"github.com/pressly/goose/v3/lock"
)

const (
	startupTestSecret = "startup-test-only-session-secret-32b"
	// Same synthetic bcrypt hash the cmd/permitpal tests use; the child never
	// authenticates anyone.
	startupTestUserHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
)

// syncBuffer lets the test read child output while the child is still writing.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// buildPermitpal compiles the real binary. These process-level tests live in
// the migrate package so their advisory-lock contention cannot race other
// migration tests in a parallel test package.
func buildPermitpal(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "permitpal")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/permitpal") // #nosec G204 -- fixed local source; only the output path varies within t.TempDir.
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build startup test binary: %v: %s", err, output)
	}
	return binary
}

// childDatabaseURL names the child's connections appName so the test can find
// them in pg_stat_activity. application_name is set in the connection string
// itself because a value already present there (from PERMITPAL_TEST_DATABASE_URL)
// would override the PGAPPNAME environment variable.
func childDatabaseURL(t *testing.T, databaseURL, appName string) string {
	t.Helper()
	childURL, err := testdb.SetConnParam(databaseURL, "application_name", appName)
	if err != nil {
		t.Fatal(err)
	}
	return childURL
}

// startPermitpal starts the binary against databaseURL with synthetic app
// secrets only, so it cannot read local or deployment configuration.
func startPermitpal(ctx context.Context, t *testing.T, binary, databaseURL, port string) (*exec.Cmd, *syncBuffer) {
	t.Helper()
	command := exec.CommandContext(ctx, binary) // #nosec G204 -- binary was built from this repository into t.TempDir.
	command.Env = []string{
		"APP_ENV=development",
		"DATA_STORE=postgres",
		"SECURE_COOKIES=false",
		"DATABASE_URL=" + databaseURL,
		"SESSION_SECRET=" + startupTestSecret,
		"PERMITPAL_USERS=driver:" + startupTestUserHash,
		"PORT=" + port,
		"METRICS_ADDR=off",
	}
	output := &syncBuffer{}
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	return command, output
}

func TestSIGTERMDuringMigrationLockWaitExitsNonzero(t *testing.T) {
	pool, databaseURL := testdb.NewEmptyWithURL(t)
	binary := buildPermitpal(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	holder, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_lock($1)", lock.DefaultLockID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = holder.Exec(cleanupCtx, "SELECT pg_advisory_unlock($1)", lock.DefaultLockID)
	}()

	childURL := childDatabaseURL(t, databaseURL, "permitpal_startup_signal_test")
	command, output := startPermitpal(ctx, t, binary, childURL, freePort(t))
	waited := false
	defer func() {
		if !waited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname = current_database() AND application_name = 'permitpal_startup_signal_test' AND query LIKE 'SELECT pg_try_advisory_lock%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("child did not reach migration lock waiting: %s", output)
		case <-ticker.C:
		}
	}
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	err = command.Wait()
	waited = true
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("startup exit = %v, want exit code 1: %s", err, output)
	}
	if !strings.Contains(output.String(), "migrate: startup interrupted: context canceled") {
		t.Fatalf("startup did not log a clear cancellation reason: %s", output)
	}
	if strings.Contains(output.String(), childURL) || strings.Contains(output.String(), startupTestSecret) {
		t.Fatal("startup log included credentials")
	}
}

func TestStartupMigratesVersion7AndServes(t *testing.T) {
	pool, databaseURL := testdb.NewEmptyWithURL(t)
	binary := buildPermitpal(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The state the production database was in before BOB-310 shipped.
	upTo7 := migrationFS(t, func(name string) bool { return name < "008" }, nil)
	if err := applyIsolated(t, ctx, pool, upTo7); err != nil {
		t.Fatalf("apply through 007: %v", err)
	}
	if got := appliedVersions(t, pool); len(got) != 7 {
		t.Fatalf("versions = %v, want 1..7", got)
	}

	port := freePort(t)
	childURL := childDatabaseURL(t, databaseURL, "permitpal_startup_serve_test")
	command, output := startPermitpal(ctx, t, binary, childURL, port)
	exited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = command.Wait()
		close(exited)
	}()
	defer func() {
		_ = command.Process.Signal(syscall.SIGTERM)
		select {
		case <-exited:
		case <-time.After(15 * time.Second):
			_ = command.Process.Kill()
			<-exited
		}
	}()

	client := &http.Client{Timeout: time.Second}
	for _, path := range []string{"/health", "/ready"} {
		url := "http://127.0.0.1:" + port + path
		deadline := time.Now().Add(20 * time.Second)
		for {
			resp, err := client.Get(url)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					break
				}
				err = fmt.Errorf("status %d", resp.StatusCode)
			}
			select {
			case <-exited:
				t.Fatalf("permitpal exited before %s responded: %v: %s", path, waitErr, output)
			default:
			}
			if time.Now().After(deadline) {
				t.Fatalf("GET %s: %v: %s", path, err, output)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	got := appliedVersions(t, pool)
	if len(got) != embeddedCount(t) || got[len(got)-1] != 8 {
		t.Fatalf("versions after startup = %v, want 1..8", got)
	}
	if !strings.Contains(output.String(), "008_road_test_date.sql") {
		t.Fatalf("startup did not log migration 008: %s", output)
	}
	if strings.Contains(output.String(), childURL) || strings.Contains(output.String(), startupTestSecret) {
		t.Fatal("startup log included credentials")
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
}
