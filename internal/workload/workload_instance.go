package workload

import (
	"errors"
	"fmt"

	"github.com/mitchellh/mapstructure"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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
			notifPayload, err := workloadInstance.NotificationPayload(
				notif.Operation,
				true,
				requeueDelay,
			)
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
				// check if error is 404 - if object no longer exists, no need to requeue
				if errors.Is(err, client.ErrorObjectNotFound) {
					log.Error(err, "object no longer exists - halting reconciliation")
					r.ReleaseLock(&workloadInstance)
					continue
				}
				if err != nil {
					log.Error(err, "failed to get workload instance by ID from API")
					r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
					continue
				}
				workloadInstance = *latestWorkloadInstance
			}

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if err := workloadInstanceCreated(r, &workloadInstance); err != nil {
					log.Error(
						err, "failed to reconcile created workload instance object",
						"workloadDefinitionID", *workloadInstance.WorkloadDefinitionID,
					)
					r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
					continue
				}
			case notifications.NotificationOperationDeleted:
				if err := workloadInstanceDeleted(r, &workloadInstance); err != nil {
					log.Error(
						err, "failed to reconcile deleted workload instance object",
						"workloadDefinitionID", *workloadInstance.WorkloadDefinitionID,
					)
					r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
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
				"workloadInstanceID", workloadInstance.ID,
			)

			log.Info("workload instance successfully reconciled", "workloadInstanceID", workloadInstance.ID)
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}

// workloadInstanceCreated performs reconciliation when a workload instance
// has been created.
func workloadInstanceCreated(
	r *controller.Reconciler,
	workloadInstance *v0.WorkloadInstance,
) error {
	// ensure workload definition is reconciled before working on an instance
	// for it
	reconciled, err := confirmWorkloadDefReconciled(r, workloadInstance)
	if err != nil {
		return fmt.Errorf("failed to determine if workload definition is reconciled: %w", err)
	}
	if !reconciled {
		return errors.New("workload definition not reconciled")
	}

	// use workload definition ID to get workload resource definitions
	workloadResourceDefinitions, err := client.GetWorkloadResourceDefinitionsByWorkloadDefinitionID(
		*workloadInstance.WorkloadDefinitionID,
		r.APIServer,
		"",
	)
	if err != nil {
		return fmt.Errorf("failed to get workload resource definitions by workload definition ID: %w", err)
	}
	if len(*workloadResourceDefinitions) == 0 {
		return errors.New("zero workload resource definitions to deploy")
	}

	// get workload definition for this instance
	workloadDefinition, err := client.GetWorkloadDefinitionByID(
		*workloadInstance.WorkloadDefinitionID,
		r.APIServer,
		"",
	)
	if err != nil {
		return fmt.Errorf("failed to get workload definition for the instance being deployed: %w", err)
	}

	// construct workload resource instances
	var workloadResourceInstances []v0.WorkloadResourceInstance
	for _, wrd := range *workloadResourceDefinitions {
		wri := v0.WorkloadResourceInstance{
			JSONDefinition:     wrd.JSONDefinition,
			WorkloadInstanceID: workloadInstance.ID,
		}
		workloadResourceInstances = append(workloadResourceInstances, wri)
	}

	// get cluster instance info
	clusterInstance, err := client.GetClusterInstanceByID(
		*workloadInstance.ClusterInstanceID,
		r.APIServer,
		"",
	)
	if err != nil {
		return fmt.Errorf("failed to get workload cluster instance by ID: %w", err)
	}

	// get a kube discovery client for the cluster
	discoveryClient, err := kube.GetDiscoveryClient(clusterInstance, true)

	// manipulate namespace on kube resources as needed
	processedWRIs, err := kube.SetNamespaces(
		&workloadResourceInstances,
		workloadInstance,
		discoveryClient,
	)
	if err != nil {
		return fmt.Errorf("failed to set namespaces for workload resource instances: %w", err)
	}

	// create a client to connect to kube API
	dynamicKubeClient, mapper, err := kube.GetClient(clusterInstance, true)
	if err != nil {
		fmt.Errorf("failed to create kube API client object: %w", err)
	}

	// create each resource in the target kube cluster
	for _, wri := range *processedWRIs {
		// marshal the resource definition json
		jsonDefinition, err := wri.JSONDefinition.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal json for workload resource instance: %w", err)
		}

		// build kube unstructured object from json
		kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
			return fmt.Errorf("failed to unmarshal json to kubernetes unstructured object: %w", err)
		}

		// set label metadata on kube object
		kubeObject, err = kube.SetLabels(
			kubeObject,
			*workloadDefinition.Name,
			*workloadInstance.Name,
		)
		if err != nil {
			return fmt.Errorf("failed to add label metadata to objects: %w", err)
		}

		// create kube resource
		_, err = kube.CreateResource(kubeObject, dynamicKubeClient, *mapper)
		if err != nil {
			return fmt.Errorf("failed to create Kubernetes resource: %w", err)
		}

		// create object in threeport API
		_, err = client.CreateWorkloadResourceInstance(
			&wri,
			r.APIServer,
			"",
		)
		if err != nil {
			return fmt.Errorf("failed to create workload resource instance in threeport: %w", err)
		}
	}

	return nil
}

// workloadInstanceDeleted performs reconciliation when a workload instance
// has been deleted
func workloadInstanceDeleted(
	r *controller.Reconciler,
	workloadInstance *v0.WorkloadInstance,
) error {
	// ensure workload definition is reconciled before working on an instance
	// for it
	reconciled, err := confirmWorkloadDefReconciled(r, workloadInstance)
	if err != nil {
		return fmt.Errorf("failed to determine if workload definition is reconciled: %w", err)
	}
	if !reconciled {
		return errors.New("workload definition not reconciled")
	}

	// get workload resource instances
	workloadResourceInstances, err := client.GetWorkloadResourceInstancesByWorkloadInstanceID(
		*workloadInstance.ID,
		r.APIServer,
		"",
	)
	if err != nil {
		return fmt.Errorf("failed to get workload resource instances by workload instance ID: %w", err)
	}
	if len(*workloadResourceInstances) == 0 {
		return errors.New("zero workload resource instances to delete")
	}

	// get cluster instance info
	clusterInstance, err := client.GetClusterInstanceByID(
		*workloadInstance.ClusterInstanceID,
		r.APIServer,
		"",
	)
	if err != nil {
		return fmt.Errorf("failed to get workload cluster instance by ID: %w", err)
	}

	// create a client to connect to kube API
	dynamicKubeClient, mapper, err := kube.GetClient(clusterInstance, true)
	if err != nil {
		fmt.Errorf("failed to create kube API client object: %w", err)
	}

	// delete each resource instance in the target kube cluster
	for _, wri := range *workloadResourceInstances {
		// marshal the resource instance json
		jsonDefinition, err := wri.JSONDefinition.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal json for workload resource instance with ID %d: %w", wri.ID, err)
		}

		// build kube unstructured object from json
		kubeObject := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := kubeObject.UnmarshalJSON(jsonDefinition); err != nil {
			return fmt.Errorf("failed to unmarshal json to kubernetes unstructured object workload resource instance with ID %d: %w", wri.ID, err)
		}

		// delete kube resource
		if err := kube.DeleteResource(kubeObject, dynamicKubeClient, *mapper); err != nil {
			return fmt.Errorf("failed to delete Kubernetes resource workload resource instance with ID %d: %w", wri.ID, err)
		}
	}

	return nil
}

// confirmWorkloadDefReconciled confirms the workload definition related to a
// workload instance is reconciled.
func confirmWorkloadDefReconciled(
	r *controller.Reconciler,
	workloadInstance *v0.WorkloadInstance,
) (bool, error) {
	workloadDefinition, err := client.GetWorkloadDefinitionByID(
		*workloadInstance.WorkloadDefinitionID,
		r.APIServer,
		"",
	)
	if err != nil {
		return false, fmt.Errorf("failed to get workload definition by workload definition ID: %w", err)
	}
	if workloadDefinition.Reconciled != nil && *workloadDefinition.Reconciled != true {
		return false, nil
	}

	return true, nil
}
