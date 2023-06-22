package controller

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/threeport/threeport/internal/agent/notify"
)

// addEventEventHandlers adds event handlers for Event objects filtered by
// resource unique ID.
func (r *ThreeportWorkloadReconciler) addEventEventHandlers(
	log logr.Logger,
	resourceUID string,
	workloadInstanceID uint,
	workloadResourceInstanceID uint,
	informer cache.SharedInformer,
) {
	// add handlers for when events are added and delet
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			event := obj.(*corev1.Event)
			if string(event.InvolvedObject.UID) == resourceUID {
				var eventSummary notify.EventSummary
				if workloadResourceInstanceID != 0 {
					eventSummary = notify.EventSummary{
						EventUID:                   string(event.ObjectMeta.UID),
						WorkloadInstanceID:         workloadInstanceID,
						WorkloadResourceInstanceID: workloadResourceInstanceID,
						ObjectNamespace:            event.InvolvedObject.Namespace,
						ObjectKind:                 event.InvolvedObject.Kind,
						ObjectName:                 event.InvolvedObject.Name,
						Timestamp:                  event.LastTimestamp,
						Type:                       event.Type,
						Reason:                     event.Reason,
						Message:                    event.Message,
					}
				} else {
					eventSummary = notify.EventSummary{
						EventUID:           string(event.ObjectMeta.UID),
						WorkloadInstanceID: workloadInstanceID,
						ObjectNamespace:    event.InvolvedObject.Namespace,
						ObjectKind:         event.InvolvedObject.Kind,
						ObjectName:         event.InvolvedObject.Name,
						Timestamp:          event.LastTimestamp,
						Type:               event.Type,
						Reason:             event.Reason,
						Message:            event.Message,
					}
				}
				threeportNotif := notify.ThreeportNotif{
					Event: &eventSummary,
				}
				*r.NotifChan <- threeportNotif
			}
		},
	}
	informer.AddEventHandler(handlers)
	log.Info(
		"event handlers for resource involved events added",
		"resourceID", resourceUID,
		"workloadInstanceID", &workloadInstanceID,
		"workloadResourceInstanceID", &workloadResourceInstanceID,
	)
}
