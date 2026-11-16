package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pressly/goose/v3/lock"
)

// StartupTimeout is the deadline Apply gives all Goose work: connection
// acquisition, lock waiting, the migrations themselves and the final version
// read. repository.NewStore adds its connect budget to it.
const StartupTimeout = 20 * time.Second

// migrationTimeouts separates the overall startup deadline from lock and
// cleanup limits. lockID selects the Postgres advisory lock; zero means Goose's
// default ID, which production always uses so every replica (and the goose CLI
// behind `make migrate`) contends on the same lock. Tests set a unique ID so
// concurrently running test packages cannot hold or wait on each other's lock.
type migrationTimeouts struct {
	total, lock, unlock time.Duration
	lockID              int64
}

// sessionLockID returns the advisory lock ID, defaulting to Goose's.
func (t migrationTimeouts) sessionLockID() int64 {
	if t.lockID == 0 {
		return lock.DefaultLockID
	}
	return t.lockID
}

// The startup budget fits the Swarm healthcheck start period (30s, also the
// Dockerfile's HEALTHCHECK --start-period). One shared 30s deadline, set in
// repository.NewStore, covers connecting (at most 10s), Apply (at most
// StartupTimeout, 20s: lock wait, migrations and the final version read) and the
// post-migration schema check. A replica waiting on another replica's migration
// gives up after 15s of lock wait, well inside the total, and exits non-zero so
// Swarm rolls the update back instead of holding a replica that never serves.
// The unlock allowance is cleanup outside that deadline: it runs after the work
// is done or abandoned, with its own 5s limit, so exit can come up to 5s after
// the 30s. It takes milliseconds on success and only runs to its limit when the
// connection is already broken.
var startupTimeouts = migrationTimeouts{
	total:  StartupTimeout,
	lock:   15 * time.Second,
	unlock: 5 * time.Second,
}

// deadlineLocker bounds Goose lock queries and its detached cleanup context.
type deadlineLocker struct {
	locker   lock.SessionLocker
	timeouts migrationTimeouts
}

// SessionLock limits acquisition to the smaller of the startup and lock budgets.
func (l deadlineLocker) SessionLock(ctx context.Context, conn *sql.Conn) error {
	ctx, cancel := context.WithTimeout(ctx, l.timeouts.lock)
	defer cancel()
	if err := l.locker.SessionLock(ctx, conn); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("migration lock wait interrupted: %w", ctx.Err())
		}
		return err
	}
	return nil
}

// SessionUnlock allows cleanup after cancellation, but only for its own bounded budget.
func (l deadlineLocker) SessionUnlock(ctx context.Context, conn *sql.Conn) error {
	// Goose detaches its cleanup context. Add an explicit deadline so a failed
	// unlock cannot extend startup by its default one-minute retry budget.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), l.timeouts.unlock)
	defer cancel()
	return l.locker.SessionUnlock(ctx, conn)
}

// retryCount rounds a budget up to one-second retry intervals, with at least
// one retry as required by Goose. The context deadline remains authoritative;
// rounding up prevents the retry limit from ending a fractional budget early.
func retryCount(budget time.Duration) uint64 {
	if budget <= 0 {
		return 1
	}
	count := budget / time.Second
	if budget%time.Second != 0 {
		count++
	}
	return uint64(count)
}
