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

// TestIsSerializationFailureClassifies covers which errors RetryWrite reruns.
func TestIsSerializationFailureClassifies(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "pg error with 40001 code is retryable",
			err:      &pgconn.PgError{Code: "40001"},
			expected: true,
		},
		{
			name:     "error text mentioning 40001 is retryable",
			err:      errors.New("TransactionRetryWithProtoRefreshError: ... RETRY_SERIALIZABLE (SQLSTATE 40001)"),
			expected: true,
		},
		{
			name:     "pg error with another code is not retryable",
			err:      &pgconn.PgError{Code: "23505"},
			expected: false,
		},
		{
			name:     "pg error with another code and 40001 in text is not retryable",
			err:      &pgconn.PgError{Code: "23505", Message: "value host-40001 already exists"},
			expected: false,
		},
		{
			name:     "untyped error with 40001 in data is not retryable",
			err:      errors.New("failed to provision host-40001"),
			expected: false,
		},
		{
			name:     "unrelated error is not retryable",
			err:      errors.New("record not found"),
			expected: false,
		},
		{
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

// TestRetryWriteReRunsThenSucceeds covers a 40001 that succeeds on the second try.
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

// TestRetryWriteSkipsNonRetryable covers a non-40001 error returned on the first try.
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

// TestRetryWriteExhaustsBudget covers a conflict that never clears.
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

	assert.Less(t, time.Since(lastAttempt), serializationRetryMaxDelay/5)
}

// TestRetryWriteStopsOnCancelledContext covers a caller that hangs up mid-write.
func TestRetryWriteStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	result := RetryWrite(ctx, func() *gorm.DB {
		calls++
		cancel()
		return &gorm.DB{Error: &pgconn.PgError{Code: "40001"}}
	})

	assert.Equal(t, 1, calls)
	assert.True(t, isSerializationFailure(result.Error))
}
