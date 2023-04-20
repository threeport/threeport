package reconcile

import (
	"errors"
	"fmt"

	"github.com/mitchellh/mapstructure"

	//kubecluster "github.com/threeport/threeport/internal/cluster/kube"
	//kubeworkload "github.com/threeport/threeport/internal/workload/kube"

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
				if err := WorkloadInstanceCreated(r, &workloadInstance); err != nil {
					log.Error(
						err, "failed to reconcile created workload instance object",
						"workloadDefinitionID", *workloadInstance.WorkloadDefinitionID,
					)
					r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
					continue
				}
			case notifications.NotificationOperationDeleted:
				if err := WorkloadInstanceDeleted(r, &workloadInstance); err != nil {
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
