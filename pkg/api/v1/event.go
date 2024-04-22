package v1

import "time"

type Event struct {

	// The object that this event refers to.
	AttachedObject *AttachedObjectReference

	// A short, machine understandable string that gives the reason for the event being generated.
	Reason string `validate:"optional"`

	// A human-readable description of the status of this operation.
	Message string `validate:"optional"`

	// The component reporting this event. Should be a short machine understandable string.
	Source string `validate:"optional"`

	// The time at which the event was first recorded.
	FirstTimestamp time.Time `validate:"optional"`

	// The time at which the most recent occurrence of this event was recorded.
	LastTimestamp time.Time `validate:"optional"`

	// The number of times this event has occurred.
	Count int32 `validate:"optional"`

	// Type of this event (Normal, Warning), new types could be added in the future.
	Type string `validate:"optional"`

	// Time when this Event was first observed.
	EventTime time.Time `validate:"optional"`

	// Data about the Event series this event represents or nil if it's a singleton Event.
	// Series *EventSeries `validate:"optional"`

	// What action was taken/failed regarding to the Regarding object.
	Action string `validate:"optional"`

	// Optional secondary object for more complex actions.
	// +optional
	// Related *ObjectReference `validate:"optional"`

	// Name of the controller that emitted this Event, e.g. `kubernetes.io/kubelet`.
	ReportingController string `validate:"optional"`

	// ID of the controller instance, e.g. `kubelet-xyzf`.
	ReportingInstance string `validate:"optional"`
}
