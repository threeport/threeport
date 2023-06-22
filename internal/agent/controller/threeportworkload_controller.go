/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/threeport/threeport/internal/agent"
	"github.com/threeport/threeport/internal/agent/notify"
	api "github.com/threeport/threeport/pkg/agent/api/v1alpha1"
)

// ThreeportWorkloadReconciler reconciles a ThreeportWorkload object
type ThreeportWorkloadReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	ManagerContext    context.Context
	RESTMapper        meta.RESTMapper
	KubeClient        *kubernetes.Clientset
	DynamicClient     *dynamic.DynamicClient
	RESTConfig        *rest.Config
	NotifChan         *chan notify.ThreeportNotif
	InformerStopChans []InformerStopChannels
}

type InformerStopChannels struct {
	WorkloadInstanceID uint
	StopChannels       []chan struct{}
}

//+kubebuilder:rbac:groups=control-plane.threeport.io,resources=threeportworkloads,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=control-plane.threeport.io,resources=threeportworkloads/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=control-plane.threeport.io,resources=threeportworkloads/finalizers,verbs=update

// Reconcile reconciles state for ThreeportWorkload resources.  The configuration
// of a ThreeportWorkload resource provides a set of resources that constitute a
// threeport workload instance.  This triggers the reconciler to start watches
// on all those resources and to watch events related to those resources.  These
// watches are used to send information back to the threeport API so the control
// plane can provide data to threeport users and allow threeport controllers to
// act upon changes in state within the Kubernetes cluster where the workload
// instance resources live.
func (r *ThreeportWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// get the ThreeportWorkload resource
	var threeportWorkload api.ThreeportWorkload
	if err := r.Get(ctx, req.NamespacedName, &threeportWorkload); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// TODO: Currently, when a controller is restarted it collects all events
	// and sends them to the threeport API and duplicates the existing events
	// there.  To handle this, we would need to start adding those UIDs to the status
	// of a ThreeportWorkload resource to persist them - and then check that
	// list before sending new events.  The problem this presents is that any
	// event that occurs for a watched object while the controller is down will
	// be missed.  The only remedy for this is to include each _Events_ UID in
	// the event record in the threeport API and remove duplicates that way.
	// For that reason, a controller in the threeport control plane should
	// actually handle de-duplication since it can do so more efficiently.

	// loop over each resource defined, place a watch on each and add informer
	// event handlers to process K8s events that involve these resources
	for _, workloadResourceInstance := range threeportWorkload.Spec.WorkloadResourceInstances {
		gvk := schema.GroupVersionKind{
			Group:   workloadResourceInstance.Group,
			Version: workloadResourceInstance.Version,
			Kind:    workloadResourceInstance.Kind,
		}

		// get resource mapping from API
		mapping, err := r.RESTMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get rest mapping for resource: %w", err)
		}
		gvr := mapping.Resource

		// get resource unique ID from API
		nsName := types.NamespacedName{
			Namespace: workloadResourceInstance.Namespace,
			Name:      workloadResourceInstance.Name,
		}
		var unstructuredObj unstructured.Unstructured
		unstructuredObj.SetGroupVersionKind(gvr.GroupVersion().WithKind(workloadResourceInstance.Kind))
		if err := r.Get(ctx, nsName, &unstructuredObj); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get resource from API: %w", err)
		}
		resourceUID := string(unstructuredObj.GetUID())

		// initiate watch on workload instance resource
		go r.watchResource(
			log,
			gvr,
			threeportWorkload.Spec.WorkloadInstanceID,
			workloadResourceInstance.Name,
			workloadResourceInstance.Namespace,
			workloadResourceInstance.ThreeportID,
			resourceUID,
		)
	}

	// set label selector - this is used to identify pods and replicasets
	labelSelector := labels.Set(map[string]string{
		agent.WorkloadInstanceLabelKey: fmt.Sprint(threeportWorkload.Spec.WorkloadInstanceID),
	}).AsSelector().String()

	// create pod and replicaset informers, add the their stop channels to the
	// reconciler, and add event handlers to the informers
	podInformer, podInformerStopChan := r.createPodInformer(ctx, labelSelector, threeportWorkload.Spec.WorkloadInstanceID)
	r.addInformerStopChannel(threeportWorkload.Spec.WorkloadInstanceID, podInformerStopChan)
	go r.addPodEventHandlers(ctx, log, threeportWorkload.Spec.WorkloadInstanceID, podInformer, podInformerStopChan)

	replicasetInformer, replicasetInformerStopChan := r.createReplicaSetInformer(ctx, labelSelector, threeportWorkload.Spec.WorkloadInstanceID)
	r.addInformerStopChannel(threeportWorkload.Spec.WorkloadInstanceID, replicasetInformerStopChan)
	go r.addReplicaSetEventHandlers(ctx, log, threeportWorkload.Spec.WorkloadInstanceID, replicasetInformer, replicasetInformerStopChan)

	return ctrl.Result{}, nil
}

// addInformerStopChannel adds an informer stop channel to an existing
// InformerStopChannels object if one exists for a particular workload instance,
// otherwise adds a new record for a workload instance with the provided stop
// channel.
func (r *ThreeportWorkloadReconciler) addInformerStopChannel(
	workloadInstanceID uint,
	stopChannel chan struct{},
) {
	workloadInstanceIDFound := false
	for i, informerStopChans := range r.InformerStopChans {
		if informerStopChans.WorkloadInstanceID == workloadInstanceID {
			informerStopChans.StopChannels = append(informerStopChans.StopChannels, stopChannel)
			r.InformerStopChans[i] = informerStopChans
			workloadInstanceIDFound = true
			break
		}
	}

	if !workloadInstanceIDFound {
		informerStopChans := InformerStopChannels{
			WorkloadInstanceID: workloadInstanceID,
			StopChannels:       []chan struct{}{stopChannel},
		}
		r.InformerStopChans = append(r.InformerStopChans, informerStopChans)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ThreeportWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.ThreeportWorkload{}).
		Complete(r)
}
