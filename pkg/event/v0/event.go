package v0

import (
	"bytes"
	"encoding/json"
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

	// Name of the controller that emitted this Event, e.g. `kubernetes.io/kubelet`.
	ReportingController string

	// ID of the controller instance, e.g. `kubelet-xyzf`.
	ReportingInstance string

	// ControllerID is the unique identifier for each controller instance.
	ControllerID string

	// ObjectType is the type of the object that this event is attached to.
	ObjectType string
}

// RecordEvent records a new event with the given information.
func (r *EventRecorder) RecordEvent(
	event *v1.Event,
	objectId *uint,
) error {
	formatString := "reason=%s&note=%s&type=%s&controllerid=%s&objectid=%d"
	formatArgs := []any{
		url.QueryEscape(*event.Reason),
		url.QueryEscape(*event.Note),
		url.QueryEscape(*event.Type),
		r.ControllerID,
		*objectId,
	}

	if event.Action != nil {
		formatString += "&action=%s"
		formatArgs = append(formatArgs, url.QueryEscape(*event.Action))
	}

	query := fmt.Sprintf(formatString, formatArgs...)
	events, err := GetEventsJoinAttachedObjectReferenceByQueryString(
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

		//TODO: use util.Operations here
		event.ReportingController = &r.ReportingController
		event.ReportingInstance = &r.ReportingInstance
		event.EventTime = util.Ptr(time.Now())
		event.LastObservedTime = util.Ptr(time.Now())
		event.Count = util.Ptr(uint(1))
		event.ControllerID = util.Ptr(r.ControllerID)
		createdEvent, err = client_v1.CreateEvent(r.APIClient, r.APIServer, event)
		if err != nil {
			return fmt.Errorf("failed to create event: %w", err)
		}

		createdAttachedObjectReference, err := client_v1.CreateAttachedObjectReference(
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

		event.AttachedObjectReferenceID = createdAttachedObjectReference.ID
		_, err = client_v1.UpdateEvent(r.APIClient, r.APIServer, event)
		if err != nil {
			return fmt.Errorf("failed to update event: %w", err)
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

// GetEventsJoinAttachedObjectReferenceByQueryString retrieves events joined to attached object reference by object ID.
func GetEventsJoinAttachedObjectReferenceByQueryString(apiClient *http.Client, apiAddr, queryString string) (*[]v1.Event, error) {
	var events []v1.Event

	response, err := client_v0.GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/events-join-attached-object-references?%s", apiAddr, client_v1.ApiVersion, queryString),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &events, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &events, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&events); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &events, nil
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
