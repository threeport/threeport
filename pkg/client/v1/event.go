package v1

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/go-logr/logr"
	v1 "github.com/threeport/threeport/pkg/api/v1"
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
	event *v1.Event,
	attachedObjectId *uint,
) error {
	formatString := ""
	formatArgs := []any{}

	if event.Reason != nil {
		formatString += "reason=%s"
		formatArgs = append(formatArgs, url.QueryEscape(*event.Reason))
	}
	if event.Note != nil {
		formatString += "&note=%s"
		formatArgs = append(formatArgs, url.QueryEscape(*event.Note))
	}
	if event.Type != nil {
		formatString += "&type=%s"
		formatArgs = append(formatArgs, url.QueryEscape(*event.Type))
	}
	if event.Action != nil {
		formatString += "&action=%s"
		formatArgs = append(formatArgs, url.QueryEscape(*event.Action))
	}

	query := fmt.Sprintf(formatString, formatArgs...)
	events, err := GetEventsByQueryString(
		r.APIClient,
		r.APIServer,
		query,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to get events by query string %s: %w",
			query,
			err,
		)
	}

	var createdEvent *v1.Event
	switch len(*events) {
	case 0:

		event.ReportingController = &r.ReportingController
		event.ReportingInstance = &r.ReportingInstance
		event.EventTime = util.Ptr(time.Now())
		event.LastObservedTime = util.Ptr(time.Now())
		event.Count = util.Ptr(uint(1))
		createdEvent, err = CreateEvent(r.APIClient, r.APIServer, event)
		if err != nil {
			return fmt.Errorf("failed to create event: %w", err)
		}

		// TODO: decide on rules for edge direction
		if err = EnsureAttachedObjectReferenceExists(
			r.APIClient,
			r.APIServer,
			r.AttachedObjectType,
			attachedObjectId,
			util.TypeName(v1.Event{}),
			createdEvent.ID,
		); err != nil {
			return fmt.Errorf("failed to ensure attached object reference exists: %w", err)
		}

	case 1:
		event = &(*events)[0]
		event.Count = util.Ptr(uint((*event.Count + 1)))
		event.LastObservedTime = util.Ptr(time.Now())
		_, err := UpdateEvent(r.APIClient, r.APIServer, event)
		if err != nil {
			return fmt.Errorf("failed to update event: %w", err)
		}
	default:
		return fmt.Errorf("unexpected number of events found: %d", len(*events))
	}

	return nil
}

// HandleEvent records the specified event
// unless the provided error is an ErrWithEvent,
// in which case it records the event provided
func (r *EventRecorder) HandleEventOverride(
	event *v1.Event,
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
			event,
			attachedObjectId,
		); err != nil {
			log.Error(err, "failed to record event")
		}
	}
}
