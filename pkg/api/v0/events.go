package v0

import "time"

const (
	// PathEventsFiltered is the list path for events filtered by subject.
	PathEventsFiltered = "/v0/events-filtered"
)

// Event is a record of an event in the system.
type Event struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// A short, machine understandable string that gives the reason for the event being generated.
	Reason *string `validate:"required" gorm:"not null;uniqueIndex:idx_events_dedup,where:deleted_at IS NULL"`

	// A human-readable description of the status of this operation.
	Note *string `validate:"optional" gorm:"uniqueIndex:idx_events_dedup,where:deleted_at IS NULL"`

	// The number of times this event has occurred.
	Count *uint `validate:"required" gorm:"not null"`

	// Time when this Event was first observed.
	EventTime *time.Time `validate:"required" gorm:"not null"`

	// The time at which the most recent occurrence of this event was recorded.
	LastObservedTime *time.Time `validate:"required" gorm:"not null"`

	// Type of this event (Normal, Warning), new types could be added in the future.
	Type *string `validate:"required" gorm:"not null;uniqueIndex:idx_events_dedup,where:deleted_at IS NULL"`

	// Name of the controller that emitted this Event.
	ReportingController *string `validate:"required" gorm:"not null;uniqueIndex:idx_events_dedup,where:deleted_at IS NULL"`

	// The fully qualified type of the object this event is about
	ObjectType *string `validate:"required" gorm:"not null;uniqueIndex:idx_events_dedup,where:deleted_at IS NULL"`
	// The id of the object this event is about
	ObjectID *uint `validate:"required" gorm:"not null;uniqueIndex:idx_events_dedup,where:deleted_at IS NULL"`
	// The name of the object this event is about, resolved on read
	ObjectName *string `validate:"optional" gorm:"-"`
}

// ExtraQueryKeys returns query parameter names that are not Event fields:
// type, namespace, version, name prefix, and reason prefix. Declaring them
// keeps the binder from rejecting a well-formed events query. They are not
// columns and never serialize into an Event response.
func (Event) ExtraQueryKeys() []string {
	return []string{
		"objecttypename",
		"objectversion",
		"objectnamespace",
		"objectnameprefix",
		"reasonprefix",
	}
}
