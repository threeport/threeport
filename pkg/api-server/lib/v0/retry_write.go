package v0

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// CockroachDB aborts a conflicting transaction with SQLSTATE 40001.
// The client retries. API writes go through RetryWrite for that.

const (
	// serializationFailureCode is SQLSTATE 40001.
	serializationFailureCode = "40001"

	// serializationRetryMax is how many times one write is attempted.
	serializationRetryMax = 12

	// serializationRetryBaseDelay is the first backoff; it doubles each attempt.
	serializationRetryBaseDelay = 10 * time.Millisecond

	// serializationRetryMaxDelay caps the backoff before jitter is applied.
	serializationRetryMaxDelay = 500 * time.Millisecond
)

// RetryWrite reruns write while CockroachDB returns 40001.
// write must be one transaction on a fresh gorm handle. A reused handle
// that already carries an error is skipped by gorm.
func RetryWrite(ctx context.Context, write func() *gorm.DB) *gorm.DB {
	var result *gorm.DB
	for attempt := 0; attempt < serializationRetryMax; attempt++ {
		result = write()
		if result.Error == nil || !isSerializationFailure(result.Error) {
			return result
		}

		if attempt == serializationRetryMax-1 {
			break
		}

		// wait, or stop if the client hung up
		timer := time.NewTimer(serializationRetryBackoff(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return result
		case <-timer.C:
		}
	}

	return result
}

// serializationRetryBackoff is exponential backoff with jitter in [0.5, 1.5).
func serializationRetryBackoff(attempt int) time.Duration {
	delay := serializationRetryBaseDelay << attempt
	if delay <= 0 || delay > serializationRetryMaxDelay {
		delay = serializationRetryMaxDelay
	}

	jitter := 0.5 + rand.Float64()
	return time.Duration(float64(delay) * jitter)
}

// isSerializationFailure reports a CockroachDB retry error (SQLSTATE 40001).
func isSerializationFailure(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == serializationFailureCode
	}

	// untyped fallback: the driver's "(SQLSTATE 40001)" form and Cockroach names
	msg := err.Error()
	return strings.Contains(msg, "(SQLSTATE "+serializationFailureCode+")") ||
		strings.Contains(msg, "RETRY_SERIALIZABLE") ||
		strings.Contains(msg, "TransactionRetryWithProtoRefreshError") ||
		strings.Contains(msg, "WriteTooOldError")
}
