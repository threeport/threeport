package reconcile

import (
	"errors"
	"fmt"

	"github.com/mitchellh/mapstructure"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	//kubecluster "github.com/threeport/threeport/internal/cluster/kube"
	//kubeworkload "github.com/threeport/threeport/internal/workload/kube"
	"github.com/threeport/threeport/internal/kube"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/controller"
	"github.com/threeport/threeport/pkg/notifications"
)

// WorkloadInstanceReconciler reconciles system state when a WorkloadInstance
// is created, updated or deleted.  It references the WorkloadResourceDefinitions
// and manages them in the configured workload cluster.
func WorkloadInstanceReconciler(r *controller.Reconciler) {
	r.ShutdownWait.Add(1)
	reconcilerLog := r.Log.WithValues("reconcilerName", r.Name)
	reconcilerLog.Info("reconciler started")
	shutdown := false

	for {
		// create a fresh log object per reconciliation loop so we don't
		// accumulate values across multiple loops
		log := r.Log.WithValues("reconcilerName", r.Name)

		if shutdown {
			break
		}

		// check for shutdown instruction
		select {
		case <-r.Shutdown:
			shutdown = true
		default:
			// pull message off queue
			msg := r.PullMessage()
			if msg == nil {
				continue
			}

			// consume message data to capture notification from API
			notif, err := notifications.ConsumeMessage(msg.Data)
			if err != nil {
				log.Error(
					err, "failed to consume message data from NATS",
					"msgSubject", msg.Subject,
					"msgData", string(msg.Data),
				)
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("workload definition reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was created
			var workloadInstance v0.WorkloadInstance
			mapstructure.Decode(notif.Object, &workloadInstance)
			log = log.WithValues(
				"workloadInstanceID", workloadInstance.ID,
				"clusterInstanceID", workloadInstance.ClusterInstanceID,
				"workloadDefinitionID", workloadInstance.WorkloadDefinitionID,
			)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.LastRequeueDelay,
				controller.DefaultInitialRequeueDelay,
				controller.DefaultMaxRequeueDelay,
			)

			// build the notif payload for requeues
			notifPayload, err := workloadInstance.NotificationPayload(true, requeueDelay)
			if err != nil {
				log.Error(err, "failed to build notification payload for requeue")
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("workload instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// check for lock on object
			locked, ok := r.CheckLock(&workloadInstance)
			if locked || !ok {
				go r.Requeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("workload instance reconciliation requeued")
				continue
			}

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&workloadInstance); !ok {
				go r.Requeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("workload instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object if requeued
			if notif.Requeue {
				latestWorkloadInstance, err := client.GetWorkloadInstanceByID(
					*workloadInstance.ID,
					r.APIServer,
					"",
				)
				if err != nil {
					log.Error(err, "failed to get workload instance by ID from API")
					r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
					continue
				}
				workloadInstance = *latestWorkloadInstance
			}

			// ensure workload definition is reconciled before working on
			// instance for it
			workloadDefinition, err := client.GetWorkloadDefinitionByID(
				*workloadInstance.WorkloadDefinitionID,
				r.APIServer,
				"",
			)
			if err != nil {
				log.Error(
					err, "failed to get workload definition by workload definition ID",
					"workloadDefinitionID", *workloadInstance.WorkloadDefinitionID,
				)
				r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
				continue
			}
			if workloadDefinition.Reconciled != nil && *workloadDefinition.Reconciled != true {
				log.V(1).Info("workload definition not yet reconciled - requeueing workload instance")
				r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
				continue
			}

			// use workload definition ID to get workload resource definitions
			workloadResourceDefinitions, err := client.GetWorkloadResourceDefinitionsByWorkloadDefinitionID(
				*workloadInstance.WorkloadDefinitionID,
				r.APIServer,
				"",
			)
			log.V(1).Info(
				"workload definitions retrieved",
				"workloadResourceDefinitions", fmt.Sprintf("%+v\n", workloadResourceDefinitions),
				"workloadInstanceID", workloadInstance.ID,
			)
			if err != nil {
				log.Error(
					err, "failed to get workload resource definitions by workload definition ID",
					"workloadDefinitionID", *workloadInstance.WorkloadDefinitionID,
				)
				r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
				continue
			}

			// get cluster instance info
			clusterInstance, err := client.GetClusterInstanceByID(
				*workloadInstance.ClusterInstanceID,
				r.APIServer,
				"",
			)
			if err != nil {
				log.Error(
					err, "failed to get workload cluster instance by ID",
					"clusterInstanceID", *workloadInstance.ClusterInstanceID,
				)
				r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
				continue
			}

			// create a client to connect to kube API
			dynamicKubeClient, mapper, err := kube.GetClient(clusterInstance, true)
			if err != nil {
				log.Error(err, "failed to create kube API client object")
				r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
				continue
			}

			// create each resource in the target kube cluster
			createSuccess := 0
			createFail := 0
			for _, wrd := range *workloadResourceDefinitions {
				wrdLog := r.Log.WithValues("workloadResourceDefinitionID", wrd.ID)

				// marshal the resource definition json
				jsonDefinition, err := wrd.JSONDefinition.MarshalJSON()
				if err != nil {
					wrdLog.Error(err, "failed to marshal the workload resource definition json")
					createFail++
					continue
				}

				// build kube unstructured object from json
				kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
				if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
					wrdLog.Error(err, "failed to unmarshal json to kubernetes unstructured object")
					createFail++
					continue
				}

				// create kube resource
				result, err := kube.CreateResource(kubeObject, dynamicKubeClient, *mapper)
				if err != nil {
					wrdLog.Error(err, "failed to create Kubernetes resource")
					createFail++
					continue
				}

				createSuccess++
				log.V(1).Info(
					"created kubernetes resource",
					"kubeResourceName", result.GetName(),
					"kubeResourceKind", result.GetKind(),
					"workloadInstanceID", workloadInstance.ID,
				)
			}

			// requeue if any kube resources failed creation
			if createFail > 0 {
				log.Error(
					errors.New("one or more resources not created"), "some Kubernetes resources failed creation",
					"resourceCreatedCount", createSuccess,
					"resourceFailedCount", createFail,
				)
				r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
				continue
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&workloadInstance); !ok {
				log.V(1).Info("workload instance remains locked - will unlock when TTL expires")
			} else {
				log.V(1).Info("workload instance unlocked")
			}

			log.V(1).Info(
				"kubernetes resource creation complete",
				"kubeResourcesCreated", createSuccess,
				"workloadInstanceID", workloadInstance.ID,
			)

			log.V(1).Info("workload instance successfully reconciled", "workloadInstanceID", workloadInstance.ID)
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
