package v0

import (
	"github.com/threeport/threeport/pkg/api/v1"
)

// ErrWithEvent is a non-recoverable error
type ErrWithEvent struct {
	// Message is the error message
	Message string

	// Event is the event that caused the error
	Event v1.Event
}

// Error returns the error message
func (e *ErrWithEvent) Error() string {
	return e.Message
}
