package v1

import (
	"time"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// Event is a record of an event in the system.
type Event struct {
	v0.Common `swaggerignore:"true" mapstructure:",squash"`

	// AttachedObjectReferenceID is a reference to an attached object.
	// A foreign key is configured via db migration in cmd/database-migrator/migrations/000010_add_events_foreign_key.go
	AttachedObjectReferenceID *uint `json:"AttachedObjectReferenceID,omitempty" query:"attachedobjectreferenceid" validate:"optional"`

	// A short, machine understandable string that gives the reason for the event being generated.
	Reason *string `json:"Reason,omitempty" query:"reason" validate:"required"`

	// A human-readable description of the status of this operation.
	Note *string `json:"Note,omitempty" query:"note" validate:"optional"`

	// The number of times this event has occurred.
	Count *uint `json:"Count,omitempty" query:"count" validate:"required"`

	// Time when this Event was first observed.
	EventTime *time.Time `json:"EventTime,omitempty" query:"eventtime" validate:"required"`

	// The time at which the most recent occurrence of this event was recorded.
	LastObservedTime *time.Time `json:"LastObservedTime,omitempty" query:"lastobservedtime" validate:"required"`

	// Type of this event (Normal, Warning), new types could be added in the future.
	Type *string `json:"Type,omitempty" query:"type" validate:"required"`

	// What action was taken/failed regarding to the Regarding object.
	Action *string `json:"Action,omitempty" query:"action" validate:"optional"`

	// Name of the controller that emitted this Event.
	ReportingController *string `json:"ReportingController,omitempty" query:"reportingcontroller" validate:"required"`
}
