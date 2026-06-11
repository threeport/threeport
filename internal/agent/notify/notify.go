package notify

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/threeport/threeport/internal/agent"
	tpapi "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	tpclient "github.com/threeport/threeport/pkg/client/v0"
	event "github.com/threeport/threeport/pkg/event/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
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
	WorkloadType                         string
	KubernetesWorkloadResourceInstanceID uint
	OperationType                        string
	OperationObject                      string
}

// EventSummary contains information collected from events related to
// threeport-managed resources.
type EventSummary struct {
	EventUID                             string
	WorkloadType                         string
	KubernetesWorkloadInstanceID         uint
	KubernetesWorkloadResourceInstanceID uint
	ObjectNamespace                      string
	ObjectKind                           string
	ObjectName                           string
	Timestamp                            metav1.Time
	Type                                 string
	Reason                               string
	Message                              string
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
// * KubernetesWorkloadInstance:
//   - consolidated status
//
// * KubernetesWorkloadResourceInstance:
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
	var workloadResourceInstances []tpapi.KubernetesWorkloadResourceInstance
	var pendingEvents []tpapi.Event

	for {
		select {
		case notif, ok := <-notifChan:
			if !ok {
				// the channel has been closed - send any pending updates to
				// threeport API and return
				log.Info("notification channel closed")
				if len(workloadResourceInstances) > 0 || len(pendingEvents) > 0 {
					// send final notifications - no point capturing any returned
					// unsent objects since this reciever is being stopped
					_, _ = sendThreeportUpdates(
						threeportAPIServer,
						threeportAPIClient,
						&workloadResourceInstances,
						&pendingEvents,
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
			// of a KubernetesWorkloadResourceInstance in which to store this info in
			// Threeport. If we want to capture this info, we'll need to add
			// that to the Threeport API data model.
			if notif.Operation != nil && notif.Operation.WorkloadType != agent.HelmWorkloadInstanceType {
				runtimeDef := datatypes.JSON([]byte(notif.Operation.OperationObject))
				workloadResourceInst := tpapi.KubernetesWorkloadResourceInstance{
					Common: tpapi.Common{
						ID: &notif.Operation.KubernetesWorkloadResourceInstanceID,
					},
					LastOperation:     &notif.Operation.OperationType,
					RuntimeDefinition: &runtimeDef,
				}
				workloadResourceInstances = appendUniqueWRI(workloadResourceInstances, workloadResourceInst)
			}
			// add events for a resource if applicable
			if notif.Event != nil {
				var evt tpapi.Event
				switch {
				case notif.Event.KubernetesWorkloadResourceInstanceID != 0:
					evt = tpapi.Event{
						Type:       util.Ptr(notif.Event.Type),
						Reason:     util.Ptr(notif.Event.Reason),
						Note:       util.Ptr(notif.Event.Message),
						ObjectType: util.Ptr(new(tpapi.KubernetesWorkloadResourceInstance).GetFullyQualifiedType()),
						ObjectID:   util.Ptr(notif.Event.KubernetesWorkloadResourceInstanceID),
					}
				case notif.Event.WorkloadType == agent.KubernetesWorkloadInstanceType:
					evt = tpapi.Event{
						Type:       util.Ptr(notif.Event.Type),
						Reason:     util.Ptr(notif.Event.Reason),
						Note:       util.Ptr(notif.Event.Message),
						ObjectType: util.Ptr(new(tpapi.KubernetesWorkloadInstance).GetFullyQualifiedType()),
						ObjectID:   util.Ptr(notif.Event.KubernetesWorkloadInstanceID),
					}
				case notif.Event.WorkloadType == agent.HelmWorkloadInstanceType:
					evt = tpapi.Event{
						Type:       util.Ptr(notif.Event.Type),
						Reason:     util.Ptr(notif.Event.Reason),
						Note:       util.Ptr(notif.Event.Message),
						ObjectType: util.Ptr(new(tpapi.HelmWorkloadInstance).GetFullyQualifiedType()),
						ObjectID:   util.Ptr(notif.Event.KubernetesWorkloadInstanceID),
					}
				default:
					log.Info(
						"unrecognized event workload type, skipping",
						"workloadType", notif.Event.WorkloadType,
					)
					continue
				}
				pendingEvents = append(pendingEvents, evt)
			}
		default:
			if len(workloadResourceInstances) > 0 || len(pendingEvents) > 0 {
				// we have data to update in threeport API - send the updates
				// and get back any kubernetes workload resource instances or events
				// that were not sent so they can be retried later
				wris, evts := sendThreeportUpdates(
					threeportAPIServer,
					threeportAPIClient,
					&workloadResourceInstances,
					&pendingEvents,
				)
				workloadResourceInstances = *wris
				pendingEvents = *evts
			}
			// wait 10 seconds before checking notif channel again
			time.Sleep(time.Second * 10)
		}
	}
}

// sendThreeportUpdates makes the call to the threeport API to update the
// workload objects.  If there is a failure on the update return the failed
// objects back so they may be retried later.  Note that if a "not found" error
// occurs on an update to a kubernetes workload resource instance it is not sent back as it
// has been deleted.
func sendThreeportUpdates(
	tpAPIServer string,
	tpAPIClient *http.Client,
	workloadResourceInstances *[]tpapi.KubernetesWorkloadResourceInstance,
	pendingEvents *[]tpapi.Event,
) (*[]tpapi.KubernetesWorkloadResourceInstance, *[]tpapi.Event) {
	var unsentWRIs []tpapi.KubernetesWorkloadResourceInstance
	var unsentEvents []tpapi.Event

	// update kubernetes workload resource instances
	for _, wri := range *workloadResourceInstances {
		wriCopy := wri // ID gets stripped by UpdateKubernetesWorkloadResourceInstance :/
		_, err := tpclient.UpdateKubernetesWorkloadResourceInstance(
			tpAPIClient,
			tpAPIServer,
			&wri,
		)
		if err != nil && !errors.Is(err, client_lib.ErrObjectNotFound) {
			unsentWRIs = append(unsentWRIs, wriCopy)
		}
	}

	// record events via the EventsRecorder so dedup-by-content happens
	// server-side: same content + same subject bumps Count rather than
	// creating duplicate rows.
	recorder := &event.EventRecorder{
		APIClient:           tpAPIClient,
		APIServer:           tpAPIServer,
		ReportingController: "agent",
	}
	for _, evt := range *pendingEvents {
		// skip events that did not match any of the recognized subject
		// types in the caller's switch; ObjectType/ObjectID are required
		// to attach the AttachedObjectReference on create.
		if evt.ObjectType == nil || evt.ObjectID == nil {
			continue
		}
		if err := recorder.RecordEvent(&evt, *evt.ObjectID, *evt.ObjectType); err != nil {
			unsentEvents = append(unsentEvents, evt)
		}
	}

	return &unsentWRIs, &unsentEvents
}

// appendUniqueWRI looks for a kubernetes workload resource instance with a
// matching ID and, if found, replaces it.  If not found it appends the new
// kubernetes workload resource instance to the existing slice.  This ensures
// the latest operation and resource object definition are the ones sent to
// the threeport API.
func appendUniqueWRI(
	wris []tpapi.KubernetesWorkloadResourceInstance,
	newWRI tpapi.KubernetesWorkloadResourceInstance,
) []tpapi.KubernetesWorkloadResourceInstance {
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
