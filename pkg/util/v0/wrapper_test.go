package v0

import (
	"errors"
	"strings"
	"testing"
)

func TestRetry(t *testing.T) {
	t.Run("returns nil on first success", func(t *testing.T) {
		calls := 0
		err := Retry(3, 0, func() error {
			calls++
			return nil
		})
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
		if calls != 1 {
			t.Fatalf("calls=%d, want 1", calls)
		}
	})

	t.Run("retries until success then stops", func(t *testing.T) {
		calls := 0
		err := Retry(5, 0, func() error {
			calls++
			if calls < 3 {
				return errors.New("nope")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
		if calls != 3 {
			t.Fatalf("calls=%d, want 3", calls)
		}
	})

	t.Run("fails after max attempts and wraps last error", func(t *testing.T) {
		calls := 0
		last := errors.New("last")
		err := Retry(3, 0, func() error {
			calls++
			return last
		})
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if calls != 3 {
			t.Fatalf("calls=%d, want 3", calls)
		}
		if !errors.Is(err, last) {
			t.Fatalf("expected error to wrap %v, got %v", last, err)
		}
		if !strings.Contains(err.Error(), "failed after 3 attempts") {
			t.Fatalf("unexpected error message: %q", err.Error())
		}
	})
}

