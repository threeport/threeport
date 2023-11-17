package v0

import (
	"errors"
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
	errorMessages := make([]string, len(me.Errors))
	for i, err := range me.Errors {
		errorMessages[i] = err.Error()
	}
	return errors.New(strings.Join(errorMessages, "\n"))
}
