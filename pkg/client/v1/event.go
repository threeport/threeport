package v1

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_v0 "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// EventRecorder records events to the backend.
type EventRecorder struct {

	// Name of the controller that emitted this Event, e.g. `kubernetes.io/kubelet`.
	ReportingController string

	// ID of the controller instance, e.g. `kubelet-xyzf`.
	ReportingInstance string

	// AttachedObjectType is the type of the object that this event is attached to.
	AttachedObjectType string

	// APIServer is the endpoint to reach Threeport REST API.
	// format: [protocol]://[hostname]:[port]
	APIServer string

	// APIClient is the HTTP client used to make requests to the Threeport API.
	APIClient *http.Client

	// Log is the logger used to write logs.
	Log *logr.Logger
}

// Event records a new event with the given information.
func (r *EventRecorder) Event(
	reason,
	note,
	eventType,
	action string,
	attachedObjectId *uint,
) {
	events, err := client_v0.GetEventsByQueryString(
		r.APIClient,
		r.APIServer,
		fmt.Sprintf(
			"reason=%s?note=%s?type=%s?action=%s",
			reason,
			note,
			eventType,
			action,
		),
	)
	if err != nil {
		// return err
	}

	var event *v0.Event
	switch len(*events) {
	case 0:
		event = &v0.Event{
			Reason:              reason,
			Note:                note,
			Count:               1,
			Type:                eventType,
			EventTime:           time.Now(),
			LastObservedTime:    time.Now(),
			Action:              action,
			ReportingController: r.ReportingController,
			ReportingInstance:   r.ReportingInstance,
		}
		event, err = client_v0.CreateEvent(r.APIClient, r.APIServer, event)
		if err != nil {
			// return err
		}
	case 1:
		event = &(*events)[0]
		event.Count++
		event.LastObservedTime = time.Now()
		_, err := client_v0.UpdateEvent(r.APIClient, r.APIServer, event)
		if err != nil {
			// return err
		}
	default:
		// err
	}

	// TODO: decide on rules for edge direction
	if err = EnsureAttachedObjectReferenceExists(
		r.APIClient,
		r.APIServer,
		util.TypeName(v0.Event{}),
		event.ID,
		r.AttachedObjectType,
		attachedObjectId,
	); err != nil {
		// return err
	}
}
