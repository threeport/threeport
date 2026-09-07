package v0

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// The RetryWrite tests drive it with a hand-built gorm result rather than a
// database, so a write closure varies only its error. Backoff runs on the real
// clock, so exhausting the budget costs the suite about 3 seconds.

// TestIsSerializationFailureClassifies covers which errors earn a re-run: the
// driver's typed code, that code in text, and the near misses that must not.
func TestIsSerializationFailureClassifies(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			// the driver's own error, which reaches the caller with its type
			// intact and is the shape a live conflict arrives in
			name:     "pg error with 40001 code is retryable",
			err:      &pgconn.PgError{Code: "40001"},
			expected: true,
		},
		{
			// CockroachDB's retry error text, matched for an error that got
			// here with its type stripped
			name:     "error text mentioning 40001 is retryable",
			err:      errors.New("TransactionRetryWithProtoRefreshError: ... RETRY_SERIALIZABLE (SQLSTATE 40001)"),
			expected: true,
		},
		{
			// a unique violation, which the handler answers with a 409 rather
			// than running again
			name:     "pg error with another code is not retryable",
			err:      &pgconn.PgError{Code: "23505"},
			expected: false,
		},
		{
			// the typed path decides on the code and never reads the message,
			// so a value carrying the digits cannot force a retry
			name:     "pg error with another code and 40001 in text is not retryable",
			err:      &pgconn.PgError{Code: "23505", Message: "value host-40001 already exists"},
			expected: false,
		},
		{
			// the fallback looks for the parenthesized form
			// pgconn.PgError.Error prints, so digits sitting in a value
			// match nothing
			name:     "untyped error with 40001 in data is not retryable",
			err:      errors.New("failed to provision host-40001"),
			expected: false,
		},
		{
			// gorm's own ErrRecordNotFound text, which carries no driver code
			// and no retry marker
			name:     "unrelated error is not retryable",
			err:      errors.New("record not found"),
			expected: false,
		},
		{
			// a write that succeeded has nothing to run again
			name:     "nil error is not retryable",
			err:      nil,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isSerializationFailure(tc.err))
		})
	}
}

// TestRetryWriteReRunsThenSucceeds asserts a conflict costs one more attempt
// and that the caller gets the result of the attempt that landed.
func TestRetryWriteReRunsThenSucceeds(t *testing.T) {
	calls := 0
	result := RetryWrite(context.Background(), func() *gorm.DB {
		calls++
		if calls == 1 {
			return &gorm.DB{Error: &pgconn.PgError{Code: "40001"}}
		}
		return &gorm.DB{Error: nil}
	})

	assert.Equal(t, 2, calls)
	assert.NoError(t, result.Error)
}

// TestRetryWriteSkipsNonRetryable covers a failure the loop does not re-run:
// one attempt, and the error comes back unchanged.
func TestRetryWriteSkipsNonRetryable(t *testing.T) {
	nonRetryable := errors.New("duplicate key value violates unique constraint")
	calls := 0
	result := RetryWrite(context.Background(), func() *gorm.DB {
		calls++
		return &gorm.DB{Error: nonRetryable}
	})

	assert.Equal(t, 1, calls)
	assert.ErrorIs(t, result.Error, nonRetryable)
}

// TestRetryWriteExhaustsBudget asserts a conflict that never clears spends the
// whole budget and then returns without waiting again.
func TestRetryWriteExhaustsBudget(t *testing.T) {
	calls := 0
	var lastAttempt time.Time
	result := RetryWrite(context.Background(), func() *gorm.DB {
		calls++
		lastAttempt = time.Now()
		return &gorm.DB{Error: &pgconn.PgError{Code: "40001"}}
	})

	assert.Equal(t, serializationRetryMax, calls)
	assert.True(t, isSerializationFailure(result.Error))

	// a backoff after the final attempt would run at least half the capped
	// delay, so returning inside a fifth of it shows the loop broke instead
	assert.Less(t, time.Since(lastAttempt), serializationRetryMaxDelay/5)
}

// TestRetryWriteStopsOnCancelledContext covers a caller that hangs up mid-write.
func TestRetryWriteStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	// cancel inside the write, so the backoff select finds a done context
	result := RetryWrite(ctx, func() *gorm.DB {
		calls++
		cancel()
		return &gorm.DB{Error: &pgconn.PgError{Code: "40001"}}
	})

	assert.Equal(t, 1, calls)

	// the caller gets the failure that stopped the write, not a context error
	assert.True(t, isSerializationFailure(result.Error))
}
