package v0

import (
	"fmt"

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

// NewErrWithEvent creates a new non-recoverable error
func NewErrWithEvent(message string) *ErrWithEvent {
	return &ErrWithEvent{
		Message: fmt.Sprintf("Non-Recoverable - %s", message),
	}
}

// NewErrNonRecoverablef creates a new non-recoverable error
func NewErrNonRecoverablef(format string, a ...any) *ErrWithEvent {
	return NewErrWithEvent(fmt.Errorf(format, a...).Error())
}

// // BroadcastEvent broadcasts the event that caused the error
// func BroadcastEvent(recorder *v1.EventRecorder, err error, reason string) {
// 	var errWithEvent *ErrWithEvent
// 	switch {
// 	case errors.As(err, &errWithEvent):
// 		recorder.Event(errWithEvent.Event)
// 	default:
// 		fmt.Printf("Broadcasting event: %s\n", errWithEvent.Event)
// 	}
// }
