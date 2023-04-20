package reconcile

import (
	"errors"

	"github.com/mitchellh/mapstructure"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/controller"
	"github.com/threeport/threeport/pkg/notifications"
)

// WorkloadDefinitionReconciler reconciles system state when a WorkloadDefinition
// is created, updated or deleted.
func WorkloadDefinitionReconciler(r *controller.Reconciler) {
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
			var workloadDefinition v0.WorkloadDefinition
			mapstructure.Decode(notif.Object, &workloadDefinition)
			log = log.WithValues("workloadDefinitionID", workloadDefinition.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.LastRequeueDelay,
				controller.DefaultInitialRequeueDelay,
				controller.DefaultMaxRequeueDelay,
			)

			// build the notif payload for requeues
			notifPayload, err := workloadDefinition.NotificationPayload(
				notif.Operation,
				true,
				requeueDelay,
			)
			if err != nil {
				log.Error(err, "failed to build notification payload for requeue")
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("workload definition reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// check for lock on object
			locked, ok := r.CheckLock(&workloadDefinition)
			if locked || !ok {
				go r.Requeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("workload definition reconciliation requeued")
				continue
			}

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&workloadDefinition); !ok {
				go r.Requeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("workload definition reconciliation requeued")
				continue
			}

			// retrieve latest version of object if requeued
			if notif.Requeue {
				latestWorkloadDefinition, err := client.GetWorkloadDefinitionByID(
					*workloadDefinition.ID,
					r.APIServer,
					"",
				)
				if err != nil {
					log.Error(err, "failed to get workload definition by ID from API")
					r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
					continue
				}
				workloadDefinition = *latestWorkloadDefinition
			}

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				workloadResourceDefs, err := WorkloadDefinitionCreated(r, &workloadDefinition)
				if err != nil {
					log.Error(err, "failed to reconcile created workload definition object")
					r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
					continue
				}
				for _, wrd := range *workloadResourceDefs {
					log.V(1).Info(
						"workload resource definition created",
						"workloadResourceDefinitionID", wrd.ID,
					)
				}
			case notifications.NotificationOperationDeleted:
				if err := WorkloadDefinitionDeleted(r, &workloadDefinition); err != nil {
					log.Error(err, "failed to reconcile deleted workload definition objects")
					r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
				continue
			}

			// set the object's Reconciled field to true
			wdReconciled := true
			reconciledWD := v0.WorkloadDefinition{
				Common: v0.Common{
					ID: workloadDefinition.ID,
				},
				Reconciled: &wdReconciled,
			}
			updatedWD, err := client.UpdateWorkloadDefinition(
				&reconciledWD,
				r.APIServer,
				"",
			)
			if err != nil {
				log.Error(err, "failed to update workload definition to mark as reconciled")
				r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
				continue
			}
			log.V(1).Info(
				"workload definition marked as reconciled in API",
				"workloadDefinitionName", updatedWD.Name,
			)

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&workloadDefinition); !ok {
				log.V(1).Info("workload definition remains locked - will unlock when TTL expires")
			} else {
				log.V(1).Info("workload definition unlocked")
			}

			log.Info("workload definition successfully reconciled")
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
