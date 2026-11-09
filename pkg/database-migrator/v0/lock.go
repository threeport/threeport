package v0

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"
)

// TableLock is a lock held in a one-row table of the migrated database.  It
// is leased for a while and renewed while it is held, so the lock of a
// migrator that stopped without releasing it, such as one whose pod was
// killed, expires rather than stopping every later one.  A holder takes back
// a lock it already holds, as an init container restarted in the same pod
// does; see LockHolder for why only an init container shares its holder.
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

// LockHolder names the migrator that holds the migration lock.
//
// An init container is named by its pod's host name, so one restarted in the
// same pod takes back the lock its last run held rather than waiting out the
// lease.  That last run is gone: a pod runs one init container at a time.
//
// Any other run, such as an operator's, is named for itself alone.  Two runs
// on one machine share a host name, and a holder takes back a lock it already
// holds, so sharing one would let both change the schema together, and the
// first to finish would release the lock while the other still ran.
func LockHolder(initContainer bool) (string, error) {
	host, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to read the host name to name the migration lock's holder: %w", err)
	}
	if initContainer {
		return host, nil
	}

	nonce := make([]byte, 6)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to name the migration lock's holder: %w", err)
	}

	return fmt.Sprintf("%s/%d/%s", host, os.Getpid(), hex.EncodeToString(nonce)), nil
}

// InPod reports whether the migrator runs in a Kubernetes pod, where a plain
// 'up' is the API server's init container.
func InPod() bool {
	return os.Getenv("KUBERNETES_SERVICE_HOST") != ""
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
			return l.heldTooLong(ctx)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.pollEvery):
		}
	}
}

// heldTooLong says who held the lock for longer than Acquire waits.  The
// holder is read once more to say so, and that read can fail, or find the
// lock just released - neither is reported as a holder that does not exist.
func (l *TableLock) heldTooLong(ctx context.Context) error {
	var holder string
	var expiresAt time.Time
	err := l.db.QueryRowContext(ctx, fmt.Sprintf("SELECT holder, expires_at FROM %s WHERE id = 1", l.table)).Scan(&holder, &expiresAt)
	switch {
	case err == sql.ErrNoRows:
		return fmt.Errorf(
			"another migrator held the migration lock for longer than %s, and released it just as this one gave up waiting; run again",
			l.wait,
		)
	case err != nil:
		return fmt.Errorf(
			"another migrator has held the migration lock for longer than %s, and which one could not be read: %w",
			l.wait, err,
		)
	}

	return fmt.Errorf(
		"another migrator, %s, has held the migration lock for longer than %s; it expires at %s unless renewed",
		holder, l.wait, expiresAt.Format(time.RFC3339),
	)
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

// startRenewing extends the lease every renew interval until Release.  A
// lock still being renewed, as when Acquire is called again while it is held,
// keeps its one renewal: a second would outlive the Release that stops the
// first, renewing a lock nothing holds any more.
func (l *TableLock) startRenewing() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stop != nil {
		select {
		case <-l.done:
			// it stopped on its own, finding the lock another's
		default:
			return
		}
	}

	l.stop = make(chan struct{})
	l.done = make(chan struct{})
	go func(stop, done chan struct{}) {
		defer close(done)

		// Release stops a renewal under way too, so a database call that
		// hangs cannot hang Release with it
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			select {
			case <-stop:
				cancel()
			case <-ctx.Done():
			}
		}()

		ticker := time.NewTicker(l.renew)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				// a renewal that fails, or takes longer than a renewal
				// interval, is tried again on the next tick, and Check
				// refuses every change once the lease runs out; one that
				// finds the lock another's has nothing to renew
				renewCtx, renewed := context.WithTimeout(ctx, l.renew)
				result, err := l.db.ExecContext(renewCtx, fmt.Sprintf(
					"UPDATE %s SET expires_at = now() + $2::INT8 * INTERVAL '1 millisecond' WHERE id = 1 AND holder = $1",
					l.table,
				), l.holder, l.lease.Milliseconds())
				renewed()
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
