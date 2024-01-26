package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// createPodInformer creates a new shared informer for pods filtered by labels,
// creates a stop channel that is used to stop the informer when the threeport
// workload instance is deleted, runs the informer and returns the informer and
// stop channel.
func (r *ThreeportWorkloadReconciler) createPodInformer(
	ctx context.Context,
	labelSelector string,
	workloadInstanceID uint,
) (cache.SharedInformer, chan struct{}) {
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

	podInformerStopChan := make(chan struct{})
	podInformer := cache.NewSharedInformer(
		listWatcher,
		&corev1.Pod{},
		6e+11, // resync every 10 min
	)
	go podInformer.Run(podInformerStopChan)

	return podInformer, podInformerStopChan
}

// addPodEventHandlers creates a new informer to watch pods with a label
// identifying it as a part of a workload instance.  Whenever a pod is added, it
// adds an event handler for Event objects associated with that pod by UID so
// that all events for that pod are sent to threeport API.
func (r *ThreeportWorkloadReconciler) addPodEventHandlers(
	ctx context.Context,
	workloadType string,
	workloadInstanceID uint,
	podInformer cache.SharedInformer,
	podInformerStopChan chan struct{},
) {
	logger := log.FromContext(ctx)

	// podEventInformers maps pod UIDs to a stop channel that is used to stop
	// informers when the pod is deleted
	podEventInformers := make(map[string]chan struct{}) // map[resourceUID]stopChannel
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			uid := obj.(*corev1.Pod).UID
			uidFound := false
			for watchedUID, _ := range podEventInformers {
				if watchedUID == string(uid) {
					uidFound = true
					break
				}
			}
			if !uidFound {
				// create and run a new informer for events related to this
				// pod
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

				r.addEventEventHandlers(
					ctx,
					string(uid),
					workloadType,
					workloadInstanceID,
					0,
					eventInformer,
				)
				podEventInformers[string(uid)] = stopChan
			}
		},
		// when a pod is deleted stop the informer that is watching for
		// events related to that pod
		DeleteFunc: func(obj interface{}) {
			uid := obj.(*corev1.Pod).UID
			for watchedUID, stopChan := range podEventInformers {
				if watchedUID == string(uid) {
					logger.Info("pod deleted, stopping event informer")
					close(stopChan)
					delete(podEventInformers, watchedUID)
					break
				}
			}
		},
	}
	podInformer.AddEventHandler(handlers)
}
