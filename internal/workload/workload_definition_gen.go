// generated by 'threeport-sdk gen' for controller scaffolding - do not edit

package workload

import (
	"errors"
	"fmt"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// WorkloadDefinitionReconciler reconciles system state when a WorkloadDefinition
// is created, updated or deleted.
func WorkloadDefinitionReconciler(r *controller.Reconciler) {
	r.ShutdownWait.Add(1)
	reconcilerLog := r.Log.WithValues("reconcilerName", r.Name)
	reconcilerLog.Info("reconciler started")
	shutdown := false

	// create a channel to receive OS signals
	osSignals := make(chan os.Signal, 1)
	lockReleased := make(chan bool, 1)

	// register the os signals channel to receive SIGINT and SIGTERM signals
	signal.Notify(osSignals, syscall.SIGINT, syscall.SIGTERM)

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
					"msgData", string(msg.Data),
				)
				r.RequeueRaw(msg)
				log.V(1).Info("workload definition reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var workloadDefinition v0.WorkloadDefinition
			if err := workloadDefinition.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("workload definition reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("workloadDefinitionID", workloadDefinition.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(&workloadDefinition)
			if locked || ok == false {
				r.Requeue(&workloadDefinition, requeueDelay, msg)
				log.V(1).Info("workload definition reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of workload definition")
					r.UnlockAndRequeue(&workloadDefinition, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for workload definition, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&workloadDefinition); !ok {
				r.Requeue(&workloadDefinition, requeueDelay, msg)
				log.V(1).Info("workload definition reconciliation requeued")
				continue
			}

			// retrieve latest version of object
			latestWorkloadDefinition, err := client.GetWorkloadDefinitionByID(
				r.APIClient,
				r.APIServer,
				*workloadDefinition.ID,
			)
			// check if error is 404 - if object no longer exists, no need to requeue
			if errors.Is(err, client.ErrObjectNotFound) {
				log.Info(fmt.Sprintf(
					"object with ID %d no longer exists - halting reconciliation",
					*workloadDefinition.ID,
				))
				r.ReleaseLock(&workloadDefinition, lockReleased, msg, true)
				continue
			}
			if err != nil {
				log.Error(err, "failed to get workload definition by ID from API")
				r.UnlockAndRequeue(&workloadDefinition, requeueDelay, lockReleased, msg)
				continue
			}
			workloadDefinition = *latestWorkloadDefinition

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if workloadDefinition.DeletionScheduled != nil {
					log.Info("workload definition scheduled for deletion - skipping create")
					break
				}
				customRequeueDelay, err := workloadDefinitionCreated(r, &workloadDefinition, &log)
				if err != nil {
					log.Error(err, "failed to reconcile created workload definition object")
					r.UnlockAndRequeue(
						&workloadDefinition,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						&workloadDefinition,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				customRequeueDelay, err := workloadDefinitionUpdated(r, &workloadDefinition, &log)
				if err != nil {
					log.Error(err, "failed to reconcile updated workload definition object")
					r.UnlockAndRequeue(
						&workloadDefinition,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						&workloadDefinition,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				customRequeueDelay, err := workloadDefinitionDeleted(r, &workloadDefinition, &log)
				if err != nil {
					log.Error(err, "failed to reconcile deleted workload definition object")
					r.UnlockAndRequeue(
						&workloadDefinition,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("deletion requeued for future reconciliation")
					r.UnlockAndRequeue(
						&workloadDefinition,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				deletionTimestamp := util.TimePtr(time.Now().UTC())
				deletedWorkloadDefinition := v0.WorkloadDefinition{
					Common: v0.Common{ID: workloadDefinition.ID},
					Reconciliation: v0.Reconciliation{
						DeletionAcknowledged: deletionTimestamp,
						DeletionConfirmed:    deletionTimestamp,
						Reconciled:           util.BoolPtr(true),
					},
				}
				if err != nil {
					log.Error(err, "failed to update workload definition to mark as reconciled")
					r.UnlockAndRequeue(&workloadDefinition, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.UpdateWorkloadDefinition(
					r.APIClient,
					r.APIServer,
					&deletedWorkloadDefinition,
				)
				if err != nil {
					log.Error(err, "failed to update workload definition to mark as deleted")
					r.UnlockAndRequeue(&workloadDefinition, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.DeleteWorkloadDefinition(
					r.APIClient,
					r.APIServer,
					*workloadDefinition.ID,
				)
				if err != nil {
					log.Error(err, "failed to delete workload definition")
					r.UnlockAndRequeue(&workloadDefinition, requeueDelay, lockReleased, msg)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&workloadDefinition,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				reconciledWorkloadDefinition := v0.WorkloadDefinition{
					Common:         v0.Common{ID: workloadDefinition.ID},
					Reconciliation: v0.Reconciliation{Reconciled: util.BoolPtr(true)},
				}
				updatedWorkloadDefinition, err := client.UpdateWorkloadDefinition(
					r.APIClient,
					r.APIServer,
					&reconciledWorkloadDefinition,
				)
				if err != nil {
					log.Error(err, "failed to update workload definition to mark as reconciled")
					r.UnlockAndRequeue(&workloadDefinition, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"workload definition marked as reconciled in API",
					"workload definitionName", updatedWorkloadDefinition.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&workloadDefinition, lockReleased, msg, true); !ok {
				log.Error(errors.New("workload definition remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("workload definition unlocked")
			}

			log.Info(fmt.Sprintf(
				"workload definition successfully reconciled for %s operation",
				notif.Operation,
			))
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
