package notify

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	tpapi "github.com/threeport/threeport/pkg/api/v0"
	tpclient "github.com/threeport/threeport/pkg/client/v0"
	"gorm.io/datatypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ThreeportNotif is the internal object used to transfer info to the Notify
// function that sends request to the threeport API.
type ThreeportNotif struct {
	Operation *ResourceOperation
	Event     *EventSummary
}

// ResourceOperation contains information gathered from watches on
// threeport-managed resources.
type ResourceOperation struct {
	WorkloadResourceInstanceID uint
	OperationType              string
	OperationObject            string
}

// EventSummary contains information collected from events related to
// threeport-managed resources.
type EventSummary struct {
	EventUID                   string
	WorkloadInstanceID         uint
	WorkloadResourceInstanceID uint
	ObjectNamespace            string
	ObjectKind                 string
	ObjectName                 string
	Timestamp                  metav1.Time
	Type                       string
	Reason                     string
	Message                    string
}

// Notify collects information about all resources being watched and
// consolidates it into Threeport objects, then sends that info to the Threeport
// API.  Info colected:
//
// * Watch operations, e.g. ADDED, MODIFIED, DELETED
// * Runtime objects, i.e. the object as stored in the runtime cluster
// * Events, i.e. the K8s events where the ojbect involved is the threeport-managed  resource
//
// Threeport Objects updated:
//
// * WorkloadInstance:
//   - consolidated status
//
// * WorkloadResourceInstance:
//   - all events
//   - runtime object
//   - most recent watch operation
func Notify(
	notifChan chan ThreeportNotif,
	threeportAPIServer string,
	threeportAPIClient *http.Client,
	log logr.Logger,
	notifyWG *sync.WaitGroup,
) {
	log.Info("notification receiver started")

	// increment the wait group and signal done if function returns (when
	// notification channel is closed)
	notifyWG.Add(1)
	defer notifyWG.Done()

	// create slices to serve as payload info store accumluated notification
	// info received from notif channel
	var workloadResourceInstances []tpapi.WorkloadResourceInstance
	var workloadEvents []tpapi.WorkloadEvent

	for {
		select {
		case notif, ok := <-notifChan:
			if !ok {
				// the channel has been closed - send any pending updates to
				// threeport API and return
				log.Info("notification channel closed")
				if len(workloadResourceInstances) > 0 || len(workloadEvents) > 0 {
					if err := sendThreeportUpdates(
						threeportAPIServer,
						threeportAPIClient,
						workloadResourceInstances,
						workloadEvents,
					); err != nil {
						log.Error(err, "failed to send threeport updates after notifcation channel closed")
						return
					}
					log.Info("final notifications sent")
				}
				log.Info("notifications receiver stopping")
				return
			}
			// notif received on channel
			// add operation details received from resource watch if
			// applicable
			if notif.Operation != nil {
				runtimeDef := datatypes.JSON([]byte(notif.Operation.OperationObject))
				workloadResourceInst := tpapi.WorkloadResourceInstance{
					Common: tpapi.Common{
						ID: &notif.Operation.WorkloadResourceInstanceID,
					},
					LastOperation:     &notif.Operation.OperationType,
					RuntimeDefinition: &runtimeDef,
				}
				workloadResourceInstances = appendUniqueWRI(workloadResourceInstances, workloadResourceInst)
			}
			// add events for a resource if applicable
			if notif.Event != nil {
				var workloadEvent tpapi.WorkloadEvent
				if notif.Event.WorkloadResourceInstanceID != 0 {
					workloadEvent = tpapi.WorkloadEvent{
						RuntimeEventUID:            &notif.Event.EventUID,
						WorkloadInstanceID:         &notif.Event.WorkloadInstanceID,
						WorkloadResourceInstanceID: &notif.Event.WorkloadResourceInstanceID,
						Type:                       &notif.Event.Type,
						Reason:                     &notif.Event.Reason,
						Message:                    &notif.Event.Message,
						Timestamp:                  &notif.Event.Timestamp.Time,
					}
				} else {
					workloadEvent = tpapi.WorkloadEvent{
						RuntimeEventUID:    &notif.Event.EventUID,
						WorkloadInstanceID: &notif.Event.WorkloadInstanceID,
						Type:               &notif.Event.Type,
						Reason:             &notif.Event.Reason,
						Message:            &notif.Event.Message,
						Timestamp:          &notif.Event.Timestamp.Time,
					}
				}
				workloadEvents = append(workloadEvents, workloadEvent)
			}
		default:
			if len(workloadResourceInstances) > 0 || len(workloadEvents) > 0 {
				// we have data to update in threeport API - if we get an error
				// that is a "not found" error we ignore as this happens when a
				// workload instance is deleted in threeport
				if err := sendThreeportUpdates(
					threeportAPIServer,
					threeportAPIClient,
					workloadResourceInstances,
					workloadEvents,
				); err != nil && !errors.Is(err, tpclient.ErrorObjectNotFound) {
					log.Error(err, "failed to send threeport updates")
				} else {
					// reset the payload info to emtpy to prepare for more notif
					// info
					workloadResourceInstances = []tpapi.WorkloadResourceInstance{}
					workloadEvents = []tpapi.WorkloadEvent{}
				}
			}
			// wait 10 seconds before checking notif channel again
			time.Sleep(time.Second * 10)
		}
	}
}

// sendThreeportUpdates makes the call to the threeport API to update the
// workload objects.
func sendThreeportUpdates(
	tpAPIServer string,
	tpAPIClient *http.Client,
	workloadResourceInstances []tpapi.WorkloadResourceInstance,
	workloadEvents []tpapi.WorkloadEvent,
) error {
	// update workload resource instances
	for _, wri := range workloadResourceInstances {
		_, err := tpclient.UpdateWorkloadResourceInstance(
			tpAPIClient,
			tpAPIServer,
			&wri,
		)
		if err != nil {
			return fmt.Errorf("failed to update workload resource instance with ID %d: %w", wri.ID, err)
		}
	}

	// add workload events
	for _, we := range workloadEvents {
		_, err := tpclient.CreateWorkloadEvent(
			tpAPIClient,
			tpAPIServer,
			&we,
		)
		if err != nil {
			return fmt.Errorf("failed to create new workload event: %w", err)
		}
	}

	return nil
}

// appendUniqueWRI looks for a workload resource instance with a matching ID
// and, if found, replaces it.  If not found it appends the new workload
// resource instance to the existing slice.  This ensures the latest operation
// and resource object definition are the ones sent to the threeport API.
func appendUniqueWRI(
	wris []tpapi.WorkloadResourceInstance,
	newWRI tpapi.WorkloadResourceInstance,
) []tpapi.WorkloadResourceInstance {
	wriFound := false
	for i, wri := range wris {
		if wri.ID == newWRI.ID {
			wriFound = true
			wris[i] = newWRI
		}
	}
	if !wriFound {
		wris = append(wris, newWRI)
	}

	return wris
}
