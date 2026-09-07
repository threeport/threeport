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

// CockroachDB aborts a transaction it cannot place in a valid one-at-a-time
// ordering, rolls it back whole, and returns SQLSTATE 40001 asking the client
// to run it again. It defaults to SERIALIZABLE, so that happens during ordinary
// contention, where PostgreSQL's READ COMMITTED default makes contending writes
// wait instead. Every CockroachDB transaction retry error carries 40001, a read
// that hit the uncertainty interval between two nodes' clocks included:
// https://docs.cockroachlabs.com/docs/stable/transaction-retry-error-reference

const (
	// serializationFailureCode is the SQLSTATE the database returns on a
	// transaction it aborted for a retry.
	serializationFailureCode = "40001"

	// serializationRetryMax is how many attempts one write gets. Spending all
	// of them waits about 3 seconds before the conflict reaches the client.
	serializationRetryMax = 12

	// serializationRetryBaseDelay seeds the backoff, doubling on each attempt
	// and scaled by jitter.
	serializationRetryBaseDelay = 10 * time.Millisecond

	// serializationRetryMaxDelay is the ceiling the doubling stops at, before
	// jitter scales it.
	serializationRetryMaxDelay = 500 * time.Millisecond
)

// RetryWrite re-runs write while the database aborts it on a serialization
// conflict, returning the final attempt's result. write must run one
// transaction and nothing else, since a re-run repeats whatever it did outside
// the database, and must build a fresh gorm handle each call, since gorm skips
// a write once a handle carries an error.
func RetryWrite(ctx context.Context, write func() *gorm.DB) *gorm.DB {
	var result *gorm.DB
	for attempt := 0; attempt < serializationRetryMax; attempt++ {
		// run the write and hand back anything but a conflict
		result = write()
		if result.Error == nil || !isSerializationFailure(result.Error) {
			return result
		}

		// return on the last failure rather than back off with no attempt left
		if attempt == serializationRetryMax-1 {
			break
		}

		// wait out the backoff, giving up the rest of the budget if the client
		// disconnects
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

// serializationRetryBackoff returns the wait before the next attempt: the base
// delay doubled per attempt and capped, then scaled by jitter, so the value
// runs to half again the cap.
func serializationRetryBackoff(attempt int) time.Duration {
	// a large enough attempt shifts the base past what a duration holds and
	// comes back non-positive, so use serializationRetryMaxDelay for that too
	delay := serializationRetryBaseDelay << attempt
	if delay <= 0 || delay > serializationRetryMaxDelay {
		delay = serializationRetryMaxDelay
	}

	// spread the wait over half to one and a half times the delay so writers
	// that collided once do not come back in step
	jitter := 0.5 + rand.Float64()
	return time.Duration(float64(delay) * jitter)
}

// isSerializationFailure reports whether err is a transaction the database
// aborted for a retry, the one failure this package re-runs.
func isSerializationFailure(err error) bool {
	if err == nil {
		return false
	}

	// a typed driver error settles it on the code alone, so digits in its
	// message never count
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == serializationFailureCode
	}

	// fall back to text for an error that lost its type, matching the code only
	// in the driver's parenthesized form so the same digits inside a value do
	// not trigger a retry; the rest are CockroachDB's own retry markers
	msg := err.Error()
	return strings.Contains(msg, "(SQLSTATE "+serializationFailureCode+")") ||
		strings.Contains(msg, "RETRY_SERIALIZABLE") ||
		strings.Contains(msg, "TransactionRetryWithProtoRefreshError") ||
		strings.Contains(msg, "WriteTooOldError")
}
