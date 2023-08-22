package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// createReplicaSetInformer creates a new shared informer for replicasets filtered by labels,
// creates a stop channel that is used to stop the informer when the threeport
// workload instance is deleted, runs the informer and returns the informer and
// stop channel.
func (r *ThreeportWorkloadReconciler) createReplicaSetInformer(
	ctx context.Context,
	labelSelector string,
	workloadInstanceID uint,
) (cache.SharedInformer, chan struct{}) {
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

	replicasetInformerStopChan := make(chan struct{})
	replicasetInformer := cache.NewSharedInformer(
		listWatcher,
		&appsv1.ReplicaSet{},
		6e+11, // resync every 10 min
	)
	go replicasetInformer.Run(replicasetInformerStopChan)

	return replicasetInformer, replicasetInformerStopChan
}

// addReplicaSetEventHandlers creates a new informer to watch replicasets with a label
// identifying it as a part of a workload instance.  Whenever a replicaset is added, it
// adds an event handler for Event objects associated with that replicaset by UID so
// that all events for that replicaset are sent to threeport API.
func (r *ThreeportWorkloadReconciler) addReplicaSetEventHandlers(
	ctx context.Context,
	workloadInstanceID uint,
	replicasetInformer cache.SharedInformer,
	replicasetInformerStopChan chan struct{},
) {
	logger := log.FromContext(ctx)

	// replicasetEventInformers maps replicaset UIDs to a stop channel that is used to stop
	// informers when the replicaset is deleted
	replicasetEventInformers := make(map[string]chan struct{}) // map[resourceUID]stopChannel
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			uid := obj.(*appsv1.ReplicaSet).UID
			uidFound := false
			for watchedUID, _ := range replicasetEventInformers {
				if watchedUID == string(uid) {
					uidFound = true
					break
				}
			}
			if !uidFound {
				// create and run a new informer for events related to this
				// replicaset
				stopChan := make(chan struct{})
				clientset, err := kubernetes.NewForConfig(r.RESTConfig)
				if err != nil {
					logger.Error(err, "failed to create kubernetes client for event informer")
					return
				}
				listWatcher := cache.NewListWatchFromClient(
					clientset.CoreV1().RESTClient(),
					"events",
					metav1.NamespaceAll,
					fields.Everything(),
				)
				eventInformer := cache.NewSharedInformer(listWatcher, &corev1.Event{}, 6e+11) // re-sync every 10 min
				go eventInformer.Run(stopChan)

				r.addEventEventHandlers(ctx, string(uid), workloadInstanceID, 0, eventInformer)
				replicasetEventInformers[string(uid)] = stopChan
			}
		},
		// when a replicaset is deleted stop the informer that is watching for
		// events related to that replicaset
		DeleteFunc: func(obj interface{}) {
			uid := obj.(*appsv1.ReplicaSet).UID
			for watchedUID, stopChan := range replicasetEventInformers {
				if watchedUID == string(uid) {
					logger.Info("replicaset deleted, stopping event informer")
					close(stopChan)
					delete(replicasetEventInformers, watchedUID)
					break
				}
			}
		},
	}
	replicasetInformer.AddEventHandler(handlers)
}
