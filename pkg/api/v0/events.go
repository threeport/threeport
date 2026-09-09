package v0

import "time"

const (
	PathEventsJoinAttachedObjectReferences = "/v0/events-join-attached-object-references"
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

	// The event's subject: the object the event is about.
	//
	// ObjectType and ObjectID are columns on the event row. Together with
	// Reason, Note, Type, and ReportingController they form the unique
	// index idx_events_dedup, so a repeated event updates Count on the
	// existing row instead of inserting another one. An index cannot span
	// tables, which is why the subject sits on this row.
	//
	// ObjectName has no column. Read paths resolve it from ObjectID
	// through the type's name resolver.
	//
	// For an event describing a script failure on a
	// MachineRuntimeInstance named "some-host" (id 42), these hold:
	//   ObjectType = "threeport.io/v0.MachineRuntimeInstance"
	//   ObjectID   = 42
	//   ObjectName = "some-host"   (read only - ignored on create)
	// A consumer like `tptctl get events` uses them to render
	// "threeport.io/machine-runtime-instance/some-host" in the OBJECT
	// column.
	ObjectType *string `validate:"required" gorm:"not null;uniqueIndex:idx_events_dedup,where:deleted_at IS NULL"`
	ObjectID   *uint   `validate:"required" gorm:"not null;uniqueIndex:idx_events_dedup,where:deleted_at IS NULL"`
	ObjectName *string `validate:"optional" gorm:"-"`
}

// ExtraQueryKeys returns the input-only filter keys the events read
// endpoint consumes directly from the query string rather than binding
// onto an Event field: the type name, api namespace, and api version
// narrow the object_type the handler filters on, the object name prefix
// selects every subject whose name starts with a token, and the reason
// prefix narrows the reason match. Declaring them keeps the strict
// query binder from rejecting a well-formed events query as carrying
// unknown parameters. They are not columns and never serialize into an
// Event response.
func (Event) ExtraQueryKeys() []string {
	return []string{
		"objecttypename",
		"objectversion",
		"objectnamespace",
		"objectnameprefix",
		"reasonprefix",
	}
}
