package notify

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/threeport/threeport/internal/agent"
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
	WorkloadType               string
	WorkloadResourceInstanceID uint
	OperationType              string
	OperationObject            string
}

// EventSummary contains information collected from events related to
// threeport-managed resources.
type EventSummary struct {
	EventUID                   string
	WorkloadType               string
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
//
// * HelmWorkloadInstance:
//   - consolidated status
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
					// send final notifications - no point capturing any returned
					// unsent objects since this reciever is being stopped
					_, _ = sendThreeportUpdates(
						threeportAPIServer,
						threeportAPIClient,
						&workloadResourceInstances,
						&workloadEvents,
					)
					log.Info("final notifications sent")
				}
				log.Info("notifications receiver stopping")
				return
			}
			// notif received on channel
			// add operation details received from resource watch if
			// applicable
			// Note: when the workload instance type is "HelmWorkloadInstance"
			// we discard this operation since helm workloads have no equivalent
			// of a WorkloadResourceInstance in which to store this info in
			// Threeport. If we want to capture this info, we'll need to add
			// that to the Threeport API data model.
			if notif.Operation != nil && notif.Operation.WorkloadType != agent.HelmWorkloadInstanceType {
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
				switch {
				case notif.Event.WorkloadResourceInstanceID != 0:
					workloadEvent = tpapi.WorkloadEvent{
						RuntimeEventUID:            &notif.Event.EventUID,
						WorkloadInstanceID:         &notif.Event.WorkloadInstanceID,
						WorkloadResourceInstanceID: &notif.Event.WorkloadResourceInstanceID,
						Type:                       &notif.Event.Type,
						Reason:                     &notif.Event.Reason,
						Message:                    &notif.Event.Message,
						Timestamp:                  &notif.Event.Timestamp.Time,
					}
				case notif.Event.WorkloadType == agent.WorkloadInstanceType:
					workloadEvent = tpapi.WorkloadEvent{
						RuntimeEventUID:    &notif.Event.EventUID,
						WorkloadInstanceID: &notif.Event.WorkloadInstanceID,
						Type:               &notif.Event.Type,
						Reason:             &notif.Event.Reason,
						Message:            &notif.Event.Message,
						Timestamp:          &notif.Event.Timestamp.Time,
					}
				case notif.Event.WorkloadType == agent.HelmWorkloadInstanceType:
					workloadEvent = tpapi.WorkloadEvent{
						RuntimeEventUID:        &notif.Event.EventUID,
						HelmWorkloadInstanceID: &notif.Event.WorkloadInstanceID,
						Type:                   &notif.Event.Type,
						Reason:                 &notif.Event.Reason,
						Message:                &notif.Event.Message,
						Timestamp:              &notif.Event.Timestamp.Time,
					}
				}
				workloadEvents = append(workloadEvents, workloadEvent)
			}
		default:
			if len(workloadResourceInstances) > 0 || len(workloadEvents) > 0 {
				// we have data to update in threeport API - send the updates
				// and get back any workload resource instances or workload
				// events that were not sent so they can be retried later
				wris, wes := sendThreeportUpdates(
					threeportAPIServer,
					threeportAPIClient,
					&workloadResourceInstances,
					&workloadEvents,
				)
				workloadResourceInstances = *wris
				workloadEvents = *wes
			}
			// wait 10 seconds before checking notif channel again
			time.Sleep(time.Second * 10)
		}
	}
}

// sendThreeportUpdates makes the call to the threeport API to update the
// workload objects.  If there is a failure on the update return the failed
// objects back so they may be retried later.  Note that if a "not found" error
// occurs on an update to a workload resource instance it is not sent back as it
// has been deleted.
func sendThreeportUpdates(
	tpAPIServer string,
	tpAPIClient *http.Client,
	workloadResourceInstances *[]tpapi.WorkloadResourceInstance,
	workloadEvents *[]tpapi.WorkloadEvent,
) (*[]tpapi.WorkloadResourceInstance, *[]tpapi.WorkloadEvent) {
	var unsentWRIs []tpapi.WorkloadResourceInstance
	var unsentWEs []tpapi.WorkloadEvent

	// update workload resource instances
	for _, wri := range *workloadResourceInstances {
		wriCopy := wri // ID gets stripped by UpdateWorkloadResourceInstance :/
		_, err := tpclient.UpdateWorkloadResourceInstance(
			tpAPIClient,
			tpAPIServer,
			&wri,
		)
		if err != nil && !errors.Is(err, tpclient.ErrorObjectNotFound) {
			unsentWRIs = append(unsentWRIs, wriCopy)
		}
	}

	// add workload events
	for _, we := range *workloadEvents {
		_, err := tpclient.CreateWorkloadEvent(
			tpAPIClient,
			tpAPIServer,
			&we,
		)
		if err != nil {
			unsentWEs = append(unsentWEs, we)
		}
	}

	return &unsentWRIs, &unsentWEs
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
