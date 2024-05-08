package v0

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/go-logr/logr"
	v1 "github.com/threeport/threeport/pkg/api/v1"
	client_v0 "github.com/threeport/threeport/pkg/client/v0"
	client_v1 "github.com/threeport/threeport/pkg/client/v1"
	tp_errors "github.com/threeport/threeport/pkg/errors/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	// Default event reasons for crud operations.
	ReasonSuccessfulCreate = "SuccessfulCreate"
	ReasonFailedCreate     = "FailedCreate"

	ReasonSuccessfulUpdate = "SuccessfulUpdate"
	ReasonFailedUpdate     = "FailedUpdate"

	ReasonSuccessfulDelete = "SuccessfulDelete"
	ReasonFailedDelete     = "FailedDelete"

	// Default event types
	TypeNormal  = "Normal"
	TypeWarning = "Warning"
)

// EventRecorder records events to the backend.
type EventRecorder struct {

	// APIClient is the HTTP client used to make requests to the Threeport API.
	APIClient *http.Client

	// APIServer is the endpoint to reach Threeport REST API.
	// format: [protocol]://[hostname]:[port]
	APIServer string

	// Name of the controller that emitted this Event.
	ReportingController string

	// ID of the controller instance.
	ReportingInstance string

	// ObjectType is the type of the object that this event is attached to.
	ObjectType string
}

// RecordEvent records a new event with the given information.
func (r *EventRecorder) RecordEvent(
	event *v1.Event,
	objectId *uint,
) error {
	formatString := "reason=%s&note=%s&type=%s&objectid=%d"
	formatArgs := []any{
		url.QueryEscape(*event.Reason),
		url.QueryEscape(*event.Note),
		url.QueryEscape(*event.Type),
		*objectId,
	}

	query := fmt.Sprintf(formatString, formatArgs...)
	events, err := client_v0.GetEventsJoinAttachedObjectReferenceByQueryString(
		r.APIClient,
		r.APIServer,
		query,
	)
	if err != nil {
		return fmt.Errorf("failed to get events by object id %d: %w", *objectId, err)
	}

	var createdEvent *v1.Event
	switch len(*events) {
	case 0:
		// use operations abstraction to atomically create event
		// and attached object reference
		operations := util.Operations{}
		var createdAttachedObjectReference *v1.AttachedObjectReference

		event.ReportingController = &r.ReportingController
		event.EventTime = util.Ptr(time.Now())
		event.LastObservedTime = util.Ptr(time.Now())
		event.Count = util.Ptr(uint(1))
		operations.AppendOperation(util.Operation{
			Name: "event",
			Create: func() error {
				createdEvent, err = client_v1.CreateEvent(r.APIClient, r.APIServer, event)
				if err != nil {
					return fmt.Errorf("failed to create event: %w", err)
				}

				return nil
			},
			Delete: func() error {
				_, err = client_v1.DeleteEvent(r.APIClient, r.APIServer, *createdEvent.ID)
				if err != nil {
					return fmt.Errorf("failed to delete event: %w", err)
				}
				return nil
			},
		})

		operations.AppendOperation(util.Operation{
			Name: "attached object reference",
			Create: func() error {
				createdAttachedObjectReference, err = client_v1.CreateAttachedObjectReference(
					r.APIClient,
					r.APIServer,
					&v1.AttachedObjectReference{
						ObjectType:         &r.ObjectType,
						ObjectID:           objectId,
						AttachedObjectType: util.Ptr(util.TypeName(v1.Event{})),
						AttachedObjectID:   createdEvent.ID,
					},
				)
				if err != nil {
					return fmt.Errorf("failed to create attached object reference: %w", err)
				}
				return nil
			},
			Delete: func() error {
				_, err = client_v1.DeleteAttachedObjectReference(
					r.APIClient,
					r.APIServer,
					*createdAttachedObjectReference.ID,
				)
				if err != nil {
					return fmt.Errorf("failed to delete attached object reference: %w", err)
				}
				return nil
			},
		})

		operations.AppendOperation(util.Operation{
			Name: "update event",
			Create: func() error {
				event.AttachedObjectReferenceID = createdAttachedObjectReference.ID
				_, err = client_v1.UpdateEvent(r.APIClient, r.APIServer, event)
				if err != nil {
					return fmt.Errorf("failed to update event: %w", err)
				}
				return nil
			},
		})

		// execute all operations
		if err := operations.Create(); err != nil {
			return fmt.Errorf("failed to create event: %w", err)
		}
	case 1:
		event = &(*events)[0]
		event.Count = util.Ptr(uint((*event.Count + 1)))
		event.LastObservedTime = util.Ptr(time.Now())
		_, err := client_v1.UpdateEvent(r.APIClient, r.APIServer, event)
		if err != nil {
			return fmt.Errorf("failed to update event: %w", err)
		}
	default:
		return fmt.Errorf("unexpected number of events found: %d", len(*events))
	}

	return nil
}

// HandleEventOverride records the specified event
// unless the provided error is an ErrWithEvent,
// in which case it records the event provided
func (r *EventRecorder) HandleEventOverride(
	event *v1.Event,
	objectId *uint,
	err error,
	log *logr.Logger,
) {
	var errWithEvent *tp_errors.ErrWithEvent
	switch {
	case errors.As(err, &errWithEvent):
		if err := r.RecordEvent(
			&errWithEvent.Event,
			objectId,
		); err != nil {
			log.Error(err, "failed to record event")
		}
	default:
		if err := r.RecordEvent(
			event,
			objectId,
		); err != nil {
			log.Error(err, "failed to record event")
		}
	}
}

// GetSuccessReasonForOperation returns the default reason for the operation.
func GetSuccessReasonForOperation(operation notifications.NotificationOperation) string {
	switch operation {
	case notifications.NotificationOperationCreated:
		return ReasonSuccessfulCreate
	case notifications.NotificationOperationUpdated:
		return ReasonSuccessfulUpdate
	case notifications.NotificationOperationDeleted:
		return ReasonSuccessfulDelete
	default:
		return ""
	}
}
