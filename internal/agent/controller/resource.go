package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/threeport/threeport/internal/agent/notify"
)

// watchResource opens a watch on the requested resource.
func (r *ThreeportWorkloadReconciler) watchResource(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	workloadInstanceID uint,
	resourceName string,
	resourceNamespace string,
	threeportID uint,
	resourceUID string,
) {
	// add resource info to log output for this resource
	logger := log.FromContext(ctx)
	logger = logger.WithValues(
		"resource", gvr.Resource,
		"resourceName", resourceName,
		"resourceNamespace", resourceNamespace,
		"workloadResourceInstanceID", threeportID,
	)

	// use dynamic client to watch specified resource
	resourceWatch, err := r.DynamicClient.Resource(gvr).Namespace(resourceNamespace).Watch(context.TODO(), metav1.ListOptions{
		FieldSelector: "metadata.name=" + resourceName,
		Watch:         true,
	})
	if err != nil {
		logger.Error(err, "failed to create watch on resource")
	}
	logger.Info("watch on resource created")

	// create and run a new informer for events related to this watched resource
	stopChan := make(chan struct{})
	clientset, err := kubernetes.NewForConfig(r.RESTConfig)
	listWatcher := cache.NewListWatchFromClient(
		clientset.CoreV1().RESTClient(),
		"events",
		metav1.NamespaceAll,
		fields.Everything(),
	)
	informer := cache.NewSharedInformer(listWatcher, &corev1.Event{}, 6e+11) // re-sync every 10 min
	go informer.Run(stopChan)

	// add an event handler for the events informer
	r.addEventEventHandlers(ctx, resourceUID, workloadInstanceID, threeportID, informer)

	// when the manager context receives an interrupt signal to shut down the
	// threeport-agent, close the watch and the stop the informer
	go func() {
		<-r.ManagerContext.Done()
		logger.Info("threeport-agent interrupted, closing watch")
		resourceWatch.Stop()
		logger.Info("threeport-agent interrupted, stopping event informer")
		close(stopChan)
	}()

	// pull operations performed on resource from watch channel
	for op := range resourceWatch.ResultChan() {
		// catch any errors received on watch channel
		if op.Type == watch.Error {
			// we can ignore errors and return if watches are closed
			if status, ok := op.Object.(*metav1.Status); ok {
				if strings.Contains(status.Message, "response body closed") {
					logger.Info("watch error ignored - watch closed")
					return
				} else {
					logger.Error(errors.New(fmt.Sprintf("watch operation: %+v", op)), "error received from watch")
				}
			} else {
				logger.Error(errors.New(fmt.Sprintf("watch operation: %+v", op)), "error received from watch")
			}
			continue
		}

		// serialize object json
		serializer := json.NewSerializerWithOptions(
			json.DefaultMetaFactory, nil, nil, json.SerializerOptions{Yaml: false, Pretty: false, Strict: false},
		)
		objectJSON, err := runtime.Encode(serializer, op.Object)
		if err != nil {
			logger.Error(err, "failed to serialize resource object to JSON")
			continue
		}

		// create notification object and send over notif channel so that
		// threeport API gets updated
		resourceOp := notify.ResourceOperation{
			WorkloadResourceInstanceID: threeportID,
			OperationType:              string(op.Type),
			OperationObject:            string(objectJSON),
		}
		threeportNotif := notify.ThreeportNotif{
			Operation: &resourceOp,
		}
		*r.NotifChan <- threeportNotif

		// if watched resource was deleted we can close the watch and stop
		// the informer
		if op.Type == watch.Deleted {
			logger.Info("resource deleted, closing watch")
			resourceWatch.Stop()
			logger.Info("resource deleted, stopping event informer")
			close(stopChan)
		}
	}
}
