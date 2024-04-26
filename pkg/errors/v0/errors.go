package v0

import (
	"errors"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_v1 "github.com/threeport/threeport/pkg/client/v1"
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

// HandleErrWithEvent broadcasts the event that caused the error
func HandleErrWithEvent(
	recorder *client_v1.EventRecorder,
	err error,
	reason,
	note,
	eventType,
	action string,
	attachedObjectId *uint,
) {
	var errWithEvent *ErrWithEvent
	switch {
	case errors.As(err, &errWithEvent):
		recorder.Event(
			&errWithEvent.Event,
			attachedObjectId,
		)
	default:
		recorder.Event(
			&v0.Event{
				Reason: reason,
				Note:   note,
				Type:   eventType,
				Action: action,
			},
			attachedObjectId,
		)
	}
}
