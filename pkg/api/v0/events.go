package v0

import (
	"time"
)

type Event struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// A short, machine understandable string that gives the reason for the event being generated.
	Reason *string `validate:"required"`

	// A human-readable description of the status of this operation.
	Note *string `validate:"optional"`

	// The number of times this event has occurred.
	Count *uint `validate:"required"`

	// Time when this Event was first observed.
	EventTime *time.Time `validate:"required"`

	// The time at which the most recent occurrence of this event was recorded.
	LastObservedTime *time.Time `validate:"required"`

	// Type of this event (Normal, Warning), new types could be added in the future.
	Type *string `validate:"required"`

	// Data about the Event series this event represents or nil if it's a singleton Event.
	// Series *EventSeries `validate:"optional"`

	// What action was taken/failed regarding to the Regarding object.
	Action *string `validate:"required"`

	// Name of the controller that emitted this Event, e.g. `kubernetes.io/kubelet`.
	ReportingController *string `validate:"required"`

	// ID of the controller instance, e.g. `kubelet-xyzf`.
	ReportingInstance *string `validate:"required"`
}
