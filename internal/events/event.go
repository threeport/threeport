package events

import (
	"fmt"
	"net/http"
	"time"

	"github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// EventRecorder records events to the backend.
type EventRecorder struct {

	// Name of the controller that emitted this Event, e.g. `kubernetes.io/kubelet`.
	ReportingController string `validate:"optional"`

	// ID of the controller instance, e.g. `kubelet-xyzf`.
	ReportingInstance string `validate:"optional"`
}

// Event records a new event with the given information.
func (r *EventRecorder) Event(
	apiClient *http.Client,
	apiEndpoint,
	reason,
	note,
	eventType,
	action string,
) {
	events, err := client.GetEventsByQueryString(
		apiClient,
		apiEndpoint,
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

	switch len(*events) {
	case 0:
		event := &v0.Event{
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
		event, err := client.CreateEvent(apiClient, apiEndpoint, event)
		if err != nil {
			// return err
		}
	case 1:
		event := (*events)[0]
		event.Count++
		_, err := client.UpdateEvent(apiClient, apiEndpoint, &event)
		if err != nil {
			// return err
		}
	default:
		// err
	}

	// if event exists, increment its count by 1
	// else, add new event

	// util.TypeName(v1.WorkloadInstance{})
	// attachedObject := v1.AttachedObjectReference{
	// 	ObjectType: ,
	// 	ObjectID: ,
	// 	AttachedObjectType: ,
	// 	AttachedObjectID: ,
	// },
}
