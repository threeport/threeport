package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/threeport/threeport/internal/agent/notify"
)

// addEventEventHandlers adds event handlers for Event objects filtered by
// resource unique ID.
func (r *ThreeportWorkloadReconciler) addEventEventHandlers(
	ctx context.Context,
	resourceUID string,
	workloadInstanceID uint,
	workloadResourceInstanceID uint,
	informer cache.SharedInformer,
) {
	logger := log.FromContext(ctx)

	// add handlers for when events are added and deleted
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
	logger.Info(
		"event handlers for resource involved events added",
		"resourceID", resourceUID,
		"workloadResourceInstanceID", &workloadResourceInstanceID,
	)
}
