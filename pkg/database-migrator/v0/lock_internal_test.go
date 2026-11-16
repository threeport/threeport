package v0

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// holderLookupDriver answers the query that reads who holds the lock with a
// fixed outcome, as a database whose lock row is gone or unreadable would.
type holderLookupDriver struct {
	err  error
	rows [][]driver.Value
}

func (d holderLookupDriver) Open(string) (driver.Conn, error) { return holderLookupConn(d), nil }

type holderLookupConn holderLookupDriver

func (c holderLookupConn) Prepare(query string) (driver.Stmt, error) {
	return holderLookupStmt(c), nil
}
func (c holderLookupConn) Close() error              { return nil }
func (c holderLookupConn) Begin() (driver.Tx, error) { return nil, errors.New("no transactions") }

type holderLookupStmt holderLookupDriver

func (s holderLookupStmt) Close() error  { return nil }
func (s holderLookupStmt) NumInput() int { return -1 }
func (s holderLookupStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, errors.New("no writes")
}
func (s holderLookupStmt) Query([]driver.Value) (driver.Rows, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &holderLookupRows{rows: s.rows}, nil
}

type holderLookupRows struct{ rows [][]driver.Value }

func (r *holderLookupRows) Columns() []string { return []string{"holder", "expires_at"} }
func (r *holderLookupRows) Close() error      { return nil }
func (r *holderLookupRows) Next(dest []driver.Value) error {
	if len(r.rows) == 0 {
		return io.EOF
	}
	copy(dest, r.rows[0])
	r.rows = r.rows[1:]
	return nil
}

func lockReadingHolderWith(t *testing.T, name string, d holderLookupDriver) *TableLock {
	t.Helper()
	sql.Register(name, d)
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &TableLock{db: db, table: "lock_table", wait: 30 * time.Minute}
}

// TestALockHeldTooLongSaysWhoHoldsIt covers the error an Acquire that gave up
// waiting returns.  The holder is read once more to say so, and when that
// read finds no row, or fails, the error says that rather than naming an
// empty holder whose lock expires in the year 1.
func TestALockHeldTooLongSaysWhoHoldsIt(t *testing.T) {
	expiresAt := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	for _, testCase := range []struct {
		name      string
		driver    holderLookupDriver
		want      []string
		unwantRaw bool
	}{
		{
			name:   "held by another migrator",
			driver: holderLookupDriver{rows: [][]driver.Value{{"api-server-7c9d", expiresAt}}},
			want:   []string{"api-server-7c9d", "2026-10-28T12:00:00Z", "30m0s"},
		},
		{
			name:   "released just as the wait ran out",
			driver: holderLookupDriver{},
			want:   []string{"released it just as this one gave up waiting", "run again"},
		},
		{
			name:   "the holder could not be read",
			driver: holderLookupDriver{err: errors.New("connection reset")},
			want:   []string{"which one could not be read", "connection reset"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			lock := lockReadingHolderWith(t, "holder-lookup-"+strings.ReplaceAll(testCase.name, " ", "-"), testCase.driver)
			err := lock.heldTooLong(context.Background())
			if err == nil {
				t.Fatal("no error")
			}
			for _, want := range testCase.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not say %q", err, want)
				}
			}
			for _, unwanted := range []string{"0001-01-01", "migrator, ,"} {
				if strings.Contains(err.Error(), unwanted) {
					t.Errorf("error %q reports a holder that was never read", err)
				}
			}
		})
	}
}
