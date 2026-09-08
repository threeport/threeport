package v0

import "time"

const (
	PathEventsJoinAttachedObjectReferences = "/v0/events-join-attached-object-references"
)

// Event is a record of an event in the system.
type Event struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// A short, machine understandable string that gives the reason for the event being generated.
	Reason *string `json:",omitempty" validate:"required" gorm:"not null"`

	// A human-readable description of the status of this operation.
	Note *string `json:",omitempty" validate:"optional"`

	// The number of times this event has occurred.
	Count *uint `json:",omitempty" validate:"required" gorm:"not null"`

	// Time when this Event was first observed.
	EventTime *time.Time `json:",omitempty" validate:"required" gorm:"not null"`

	// The time at which the most recent occurrence of this event was recorded.
	LastObservedTime *time.Time `json:",omitempty" validate:"required" gorm:"not null"`

	// Type of this event (Normal, Warning), new types could be added in the future.
	Type *string `json:",omitempty" validate:"required" gorm:"not null"`

	// Name of the controller that emitted this Event.
	ReportingController *string `json:",omitempty" validate:"required" gorm:"not null"`

	// Fields carrying the event's subject - the object the event is
	// about. They flow in both directions:
	//   - On create: the caller sets ObjectType (fully qualified type form) + ObjectID
	//     in the request body. Event.BeforeCreate validates them;
	//     Event.AfterCreate inserts the matching AttachedObjectReference
	//     in the same transaction. ObjectName is ignored on write.
	//   - On read: GetEventsJoinAttachedObjectReferenceByQueryString
	//     projects the joined AOR's base object back into these
	//     fields, then resolves ObjectName via the type's name resolver.
	//
	// gorm:"-" keeps them off the Event row in the schema - the AOR
	// is the source of truth on disk for the subject linkage.
	//
	// For an event describing a script failure on a
	// MachineRuntimeInstance named "some-host" (id 42), these hold:
	//   ObjectType = "threeport.io/v0.MachineRuntimeInstance"
	//   ObjectID   = 42
	//   ObjectName = "some-host"   (read only - ignored on create)
	// A consumer like `tptctl get events` uses them to render
	// "threeport.io/machine-runtime-instance/some-host" in the OBJECT
	// column.
	ObjectType *string `json:",omitempty" validate:"optional" gorm:"-"`
	ObjectID   *uint   `json:",omitempty" validate:"optional" gorm:"-"`
	ObjectName *string `json:",omitempty" validate:"optional" gorm:"-"`
}

// ExtraQueryKeys returns the input-only filter keys the events-join
// read endpoint consumes directly from the query string rather than
// binding onto an Event field: the type name, api namespace, and api
// version narrow the attached-object-reference join, and the reason
// prefix narrows the reason match. Declaring them keeps the strict
// query binder from rejecting a well-formed events query as carrying
// unknown parameters. They are not columns and never serialize into an
// Event response.
func (Event) ExtraQueryKeys() []string {
	return []string{"objecttypename", "objectversion", "objectnamespace", "reasonprefix"}
}
