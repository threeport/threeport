package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"github.com/threeport/threeport/internal/agent"
	"github.com/threeport/threeport/internal/agent/notify"
)

// addEventHandler adds event handlers for Event objects filtered by resource
// unique ID.
func (r *ThreeportWorkloadReconciler) addEventHandler(
	log logr.Logger,
	resourceUID string,
	workloadInstanceID *uint,
	workloadResourceInstanceID *uint,
) {
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			event := obj.(*corev1.Event)
			if string(event.InvolvedObject.UID) == resourceUID {
				var eventSummary notify.EventSummary
				if workloadResourceInstanceID != nil {
					eventSummary = notify.EventSummary{
						EventUID:                   string(event.ObjectMeta.UID),
						WorkloadInstanceID:         *workloadInstanceID,
						WorkloadResourceInstanceID: *workloadResourceInstanceID,
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
						WorkloadInstanceID: *workloadInstanceID,
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
	r.EventInformer.AddEventHandler(handlers)
	log.Info(
		"event handlers for resource involved events added",
		"resourceID", resourceUID,
		"workloadInstanceID", &workloadInstanceID,
		"workloadResourceInstanceID", &workloadResourceInstanceID,
	)
}

// addPodEventHandler creates a new informer to watch pods with a label
// identifying it as a part of a workload instance.  Whenever a pod is added, it
// adds an event handler for Event objects associated with that pod by UID so
// that all events for that pod are sent to threeport API.
func (r *ThreeportWorkloadReconciler) addPodEventHandler(
	ctx context.Context,
	log logr.Logger,
	workloadInstanceID uint,
) {
	// set label selector
	labelSelector := labels.Set(map[string]string{
		agent.WorkloadInstanceLabelKey: fmt.Sprint(workloadInstanceID),
	}).AsSelector().String()

	// create a watch for pods based on label selector
	listWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.LabelSelector = labelSelector
			return r.KubeClient.CoreV1().Pods(corev1.NamespaceAll).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.LabelSelector = labelSelector
			return r.KubeClient.CoreV1().Pods(corev1.NamespaceAll).Watch(ctx, options)
		},
	}

	// keep track of watch pod UIDs and when a new one shows up add an event
	// handler to watch for Events resources associated with it
	var watchedUIDs []string
	_, informer := cache.NewInformer(
		listWatcher,
		&corev1.Pod{},
		6e+11, // resync every 10 min
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				uid := obj.(*corev1.Pod).UID
				uidFound := false
				for _, watchedUID := range watchedUIDs {
					if watchedUID == string(uid) {
						uidFound = true
						break
					}
				}
				if !uidFound {
					r.addEventHandler(log, string(uid), &workloadInstanceID, nil)
					watchedUIDs = append(watchedUIDs, string(uid))
				}
			},
		},
	)

	go informer.Run(r.ManagerContext.Done())
}

// addReplicaSetEventHandler creates a new informer to watch replicasets with a label
// identifying it as a part of a workload instance.  Whenever a replicaset is added, it
// adds an event handler for Event objects associated with that replicaset by UID so
// that all events for that replicaset are sent to threeport API.
func (r *ThreeportWorkloadReconciler) addReplicaSetEventHandler(
	ctx context.Context,
	log logr.Logger,
	workloadInstanceID uint,
) {
	// set label selector
	labelSelector := labels.Set(map[string]string{
		agent.WorkloadInstanceLabelKey: fmt.Sprint(workloadInstanceID),
	}).AsSelector().String()

	// create a watch for replicasets based on label selector
	listWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.LabelSelector = labelSelector
			return r.KubeClient.AppsV1().ReplicaSets(corev1.NamespaceAll).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.LabelSelector = labelSelector
			return r.KubeClient.AppsV1().ReplicaSets(corev1.NamespaceAll).Watch(ctx, options)
		},
	}

	// keep track of watch replicaset UIDs and when a new one shows up add an event
	// handler to watch for Events resources associated with it
	var watchedUIDs []string
	_, informer := cache.NewInformer(
		listWatcher,
		&appsv1.ReplicaSet{},
		6e+11, // resync every 10 min
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				uid := obj.(*appsv1.ReplicaSet).UID
				uidFound := false
				for _, watchedUID := range watchedUIDs {
					if watchedUID == string(uid) {
						uidFound = true
						break
					}
				}
				if !uidFound {
					r.addEventHandler(log, string(uid), &workloadInstanceID, nil)
					watchedUIDs = append(watchedUIDs, string(uid))
				}
			},
		},
	)

	go informer.Run(r.ManagerContext.Done())
}
