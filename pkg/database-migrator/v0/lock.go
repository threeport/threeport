package v0

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// TableLock is a lock held in a one-row table of the migrated database.  It
// is leased for a while and renewed while it is held, so the lock of a
// migrator that stopped without releasing it, such as one whose pod was
// killed, expires rather than stopping every later one.  A holder takes back
// a lock it already holds, as an init container restarted in the same pod
// does.
type TableLock struct {
	db     *sql.DB
	table  string
	holder string

	// how long a lease lasts, how often it is renewed, and how long and how
	// often Acquire tries for it
	lease     time.Duration
	renew     time.Duration
	wait      time.Duration
	pollEvery time.Duration

	mu   sync.Mutex
	stop chan struct{}
	done chan struct{}
}

// DefaultLockLease is how long a migration lock lasts without being renewed.
const DefaultLockLease = 5 * time.Minute

// NewTableLock returns a lock kept in the given table, which is created when
// it does not exist.  holder names this migrator, such as its pod's name.
func NewTableLock(ctx context.Context, db *sql.DB, table, holder string) (*TableLock, error) {
	return NewTableLockWithTiming(ctx, db, table, holder, DefaultLockLease, 30*time.Minute, 5*time.Second)
}

// NewTableLockWithTiming returns a lock with the given lease, renewed every
// third of it, for which Acquire tries for wait, every pollEvery.
func NewTableLockWithTiming(
	ctx context.Context,
	db *sql.DB,
	table, holder string,
	lease, wait, pollEvery time.Duration,
) (*TableLock, error) {
	if !identifierPattern.MatchString(table) {
		return nil, fmt.Errorf("invalid lock table name %q", table)
	}
	if holder == "" {
		return nil, fmt.Errorf("a migration lock needs a holder name")
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id INT8 PRIMARY KEY DEFAULT 1 CHECK (id = 1),
		holder STRING NOT NULL,
		expires_at TIMESTAMPTZ NOT NULL
	)`, table)); err != nil {
		return nil, fmt.Errorf("failed to create lock table %s: %w", table, err)
	}

	return &TableLock{
		db:        db,
		table:     table,
		holder:    holder,
		lease:     lease,
		renew:     lease / 3,
		wait:      wait,
		pollEvery: pollEvery,
	}, nil
}

// Acquire waits until the lock is free, expired or already this holder's,
// takes it, and keeps renewing it until Release.
func (l *TableLock) Acquire(ctx context.Context) error {
	deadline := time.Now().Add(l.wait)
	for {
		acquired, err := l.tryAcquire(ctx)
		if err != nil {
			return err
		}
		if acquired {
			l.startRenewing()
			return nil
		}

		if time.Now().After(deadline) {
			var holder string
			var expiresAt time.Time
			_ = l.db.QueryRowContext(ctx, fmt.Sprintf("SELECT holder, expires_at FROM %s WHERE id = 1", l.table)).Scan(&holder, &expiresAt)
			return fmt.Errorf(
				"another migrator, %s, has held the migration lock for longer than %s; it expires at %s unless renewed",
				holder, l.wait, expiresAt.Format(time.RFC3339),
			)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.pollEvery):
		}
	}
}

// tryAcquire takes the lock if it is free, expired or this holder's.  The
// insert either claims the row or leaves it, in one statement, so two
// migrators trying together cannot both get it.
func (l *TableLock) tryAcquire(ctx context.Context) (bool, error) {
	result, err := l.db.ExecContext(ctx, fmt.Sprintf(
		`INSERT INTO %[1]s (id, holder, expires_at) VALUES (1, $1, now() + $2::INT8 * INTERVAL '1 millisecond')
		ON CONFLICT (id) DO UPDATE SET holder = excluded.holder, expires_at = excluded.expires_at
		WHERE %[1]s.expires_at < now() OR %[1]s.holder = excluded.holder`,
		l.table,
	), l.holder, l.lease.Milliseconds())
	if err != nil {
		return false, fmt.Errorf("failed to try for the migration lock: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to check for the migration lock: %w", err)
	}

	return rows == 1, nil
}

// startRenewing extends the lease every renew interval until Release.
func (l *TableLock) startRenewing() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.stop = make(chan struct{})
	l.done = make(chan struct{})
	go func(stop, done chan struct{}) {
		defer close(done)
		ticker := time.NewTicker(l.renew)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				// a renewal that fails is tried again on the next tick,
				// and Check refuses every change once the lease runs out;
				// one that finds the lock another's has nothing to renew
				result, err := l.db.Exec(fmt.Sprintf(
					"UPDATE %s SET expires_at = now() + $2::INT8 * INTERVAL '1 millisecond' WHERE id = 1 AND holder = $1",
					l.table,
				), l.holder, l.lease.Milliseconds())
				if err != nil {
					continue
				}
				if rows, err := result.RowsAffected(); err == nil && rows == 0 {
					return
				}
			}
		}
	}(l.stop, l.done)
}

// Check returns ErrLockLost unless this holder still holds the lock with at
// least a renewal interval left on it, so a step started now cannot outlast
// the lease before the next renewal.
func (l *TableLock) Check(ctx context.Context) error {
	var held bool
	if err := l.db.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT count(*) = 1 FROM %s WHERE id = 1 AND holder = $1 AND expires_at > now() + $2::INT8 * INTERVAL '1 millisecond'",
		l.table,
	), l.holder, l.renew.Milliseconds()).Scan(&held); err != nil {
		return fmt.Errorf("%w: failed to check it: %w", ErrLockLost, err)
	}
	if !held {
		return fmt.Errorf("%w: it expired or another migrator holds it", ErrLockLost)
	}

	return nil
}

// Release stops renewing the lock and gives it up, if it is still this
// holder's.
func (l *TableLock) Release(ctx context.Context) error {
	l.mu.Lock()
	if l.stop != nil {
		close(l.stop)
		<-l.done
		l.stop = nil
	}
	l.mu.Unlock()

	if _, err := l.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = 1 AND holder = $1", l.table), l.holder); err != nil {
		return fmt.Errorf("failed to release the migration lock: %w", err)
	}

	return nil
}
