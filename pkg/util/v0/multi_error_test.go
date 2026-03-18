package v0

import (
	"errors"
	"testing"
)

func TestMultiError_Error(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		me := MultiError{}
		if err := me.Error(); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("single error returns same message", func(t *testing.T) {
		me := MultiError{}
		me.AppendError(errors.New("boom"))

		err := me.Error()
		if err == nil {
			t.Fatalf("expected non-nil error")
		}
		if got, want := err.Error(), "boom"; got != want {
			t.Fatalf("error message = %q, want %q", got, want)
		}
	})

	t.Run("multiple errors are joined with newline", func(t *testing.T) {
		me := MultiError{}
		me.AppendError(errors.New("first"))
		me.AppendError(errors.New("second"))

		err := me.Error()
		if err == nil {
			t.Fatalf("expected non-nil error")
		}
		if got, want := err.Error(), "first\nsecond"; got != want {
			t.Fatalf("error message = %q, want %q", got, want)
		}
	})
}

