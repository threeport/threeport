package v0

import (
	"errors"
	"fmt"
)

// ErrNonRecoverable is a non-recoverable error
type ErrNonRecoverable struct {
	Message string
}

// Error returns the error message
func (e *ErrNonRecoverable) Error() string {
	return e.Message
}

// NewErrNonRecoverable creates a new non-recoverable error
func NewErrNonRecoverable(message string) *ErrNonRecoverable {
	return &ErrNonRecoverable{
		Message: fmt.Sprintf("Non-Recoverable - %s", message),
	}
}

// NewErrNonRecoverablef creates a new non-recoverable error
func NewErrNonRecoverablef(format string, a ...any) *ErrNonRecoverable {
	return NewErrNonRecoverable(fmt.Errorf(format, a...).Error())
}


// IsErrRecoverable checks if an error is recoverable
func IsErrRecoverable(err error) bool {
	var errNonRecoverable *ErrNonRecoverable
	switch {
	case errors.As(err, &errNonRecoverable):
		return false
	default:
		return true
	}
}
