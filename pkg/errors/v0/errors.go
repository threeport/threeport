package v0

import (
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// ErrWithEvent is a non-recoverable error
type ErrWithEvent struct {
	// Message is the error message
	Message string

	// Event is the event that caused the error
	Event v0.Event
}

// Error returns the error message
func (e *ErrWithEvent) Error() string {
	return e.Message
}
