package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/threeport/threeport/internal/agent/notify"
)

// watchResource opens a watch on the requested resource.
func (r *ThreeportWorkloadReconciler) watchResource(
	log logr.Logger,
	gvr schema.GroupVersionResource,
	resourceName string,
	resourceNamespace string,
	threeportID uint,
) {
	// add resource info to log output for this resource
	watchLog := log.WithValues(
		"resource", gvr.Resource,
		"resourceName", resourceName,
		"resourceNamespace", resourceNamespace,
		"threeportID", threeportID,
	)

	// use dynamic client to watch specified resource
	resourceWatch, err := r.DynamicClient.Resource(gvr).Namespace(resourceNamespace).Watch(context.TODO(), metav1.ListOptions{
		FieldSelector: "metadata.name=" + resourceName,
		Watch:         true,
	})
	if err != nil {
		watchLog.Error(err, "failed to create watch on resource")
	}
	watchLog.Info("watch on resource created")

	// use a goroutine to stop the watch when an interrupt signal is received
	go func() {
		<-r.ManagerContext.Done()
		watchLog.Info("closing watch")
		resourceWatch.Stop()
	}()

	// pull operations performed on resource from watch channel
	for op := range resourceWatch.ResultChan() {
		// catch any errors received on watch channel
		if op.Type == watch.Error {
			// we can ignore errors if controller is shutting down
			if r.ManagerContext.Done() != nil {
				watchLog.Info("watch error ignored - controller shutting down")
				return
			}
			errMsg := fmt.Sprintf("watch object: %+v\n", op.Object)
			watchLog.Error(errors.New(errMsg), "error recieved on watch channel")
			continue
		}

		// serialize object json
		serializer := json.NewSerializerWithOptions(
			json.DefaultMetaFactory, nil, nil, json.SerializerOptions{Yaml: false, Pretty: false, Strict: false},
		)
		objectJSON, err := runtime.Encode(serializer, op.Object)
		if err != nil {
			watchLog.Error(err, "failed to serialize resource object to JSON")
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
	}
}
