package v0

import (
	"errors"
	"fmt"
	"strings"
)

// MultiError is an error type that contains multiple errors.
type MultiError struct {
	Errors []error
}

// AppendError adds an error to the MultiError.
func (me *MultiError) AppendError(err error) {
	me.Errors = append(me.Errors, err)
}

// Error returns a string representation of the MultiError.
func (me MultiError) Error() error {
	if len(me.Errors) == 0 {
		return nil
	}
	errorMessages := make([]string, len(me.Errors))
	for i, err := range me.Errors {
		errorMessages[i] = err.Error()
	}
	return errors.New(strings.Join(errorMessages, "\n"))
}

// UnmetPrerequisites joins independent errors and prefixes the result.
// It returns nil when every argument is nil.
func UnmetPrerequisites(prefix string, errs ...error) error {
	joined := errors.Join(errs...)
	if joined == nil {
		return nil
	}

	return fmt.Errorf("%s\n%w", prefix, joined)
}
