package v1

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_v0 "github.com/threeport/threeport/pkg/client/v0"
	tp_errors "github.com/threeport/threeport/pkg/errors/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// EventRecorder records events to the backend.
type EventRecorder struct {

	// APIClient is the HTTP client used to make requests to the Threeport API.
	APIClient *http.Client

	// APIServer is the endpoint to reach Threeport REST API.
	// format: [protocol]://[hostname]:[port]
	APIServer string

	// Name of the controller that emitted this Event, e.g. `kubernetes.io/kubelet`.
	ReportingController string

	// ID of the controller instance, e.g. `kubelet-xyzf`.
	ReportingInstance string

	// AttachedObjectType is the type of the object that this event is attached to.
	AttachedObjectType string
}

// RecordEvent records a new event with the given information.
func (r *EventRecorder) RecordEvent(
	event *v0.Event,
	attachedObjectId *uint,
) error {
	events, err := client_v0.GetEventsByQueryString(
		r.APIClient,
		r.APIServer,
		fmt.Sprintf(
			"reason=%s?note=%s?type=%s?action=%s",
			event.Reason,
			event.Note,
			event.Type,
			event.Action,
		),
	)
	if err != nil {
		return fmt.Errorf(
			"failed to get events by query string reason=%s?note=%s?type=%s?action=%s: %w",
			event.Reason,
			event.Note,
			event.Type,
			event.Action,
			err,
		)
	}

	var createdEvent *v0.Event
	switch len(*events) {
	case 0:

		event.ReportingController = r.ReportingController
		event.ReportingInstance = r.ReportingInstance
		event.EventTime = time.Now()
		event.LastObservedTime = time.Now()
		event.Count = 1
		createdEvent, err = client_v0.CreateEvent(r.APIClient, r.APIServer, event)
		if err != nil {
			return fmt.Errorf("failed to create event: %w", err)
		}
	case 1:
		event = &(*events)[0]
		event.Count++
		event.LastObservedTime = time.Now()
		_, err := client_v0.UpdateEvent(r.APIClient, r.APIServer, event)
		if err != nil {
			return fmt.Errorf("failed to update event: %w", err)
		}
	default:
		return fmt.Errorf("unexpected number of events found: %d", len(*events))
	}

	// TODO: decide on rules for edge direction
	if err = EnsureAttachedObjectReferenceExists(
		r.APIClient,
		r.APIServer,
		util.TypeName(v0.Event{}),
		createdEvent.ID,
		r.AttachedObjectType,
		attachedObjectId,
	); err != nil {
		return fmt.Errorf("failed to ensure attached object reference exists: %w", err)
	}

	return nil
}

// HandleEvent records the specified event
// unless the provided error is an ErrWithEvent,
// in which case it records the event provided
func (r *EventRecorder) HandleEventOverride(
	reason,
	note,
	eventType,
	action string,
	attachedObjectId *uint,
	err error,
	log *logr.Logger,
) {
	var errWithEvent *tp_errors.ErrWithEvent
	switch {
	case errors.As(err, &errWithEvent):
		if err := r.RecordEvent(
			&errWithEvent.Event,
			attachedObjectId,
		); err != nil {
			log.Error(err, "failed to record event")
		}
	default:
		if err := r.RecordEvent(
			&v0.Event{
				Reason: reason,
				Note:   note,
				Type:   eventType,
				Action: action,
			},
			attachedObjectId,
		); err != nil {
			log.Error(err, "failed to record event")
		}
	}
}
