// generated by 'threeport-sdk codegen controller' - do not edit

package terraform

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

// TerraformInstanceReconciler reconciles system state when a TerraformInstance
// is created, updated or deleted.
func TerraformInstanceReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("terraform instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var terraformInstance v0.TerraformInstance
			if err := terraformInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("terraform instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("terraformInstanceID", terraformInstance.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(&terraformInstance)
			if locked || ok == false {
				r.Requeue(&terraformInstance, requeueDelay, msg)
				log.V(1).Info("terraform instance reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of terraform instance")
					r.UnlockAndRequeue(&terraformInstance, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for terraform instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&terraformInstance); !ok {
				r.Requeue(&terraformInstance, requeueDelay, msg)
				log.V(1).Info("terraform instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object
			latestTerraformInstance, err := client.GetTerraformInstanceByID(
				r.APIClient,
				r.APIServer,
				*terraformInstance.ID,
			)
			// check if error is 404 - if object no longer exists, no need to requeue
			if errors.Is(err, client.ErrObjectNotFound) {
				log.Info(fmt.Sprintf(
					"object with ID %d no longer exists - halting reconciliation",
					*terraformInstance.ID,
				))
				r.ReleaseLock(&terraformInstance, lockReleased, msg, true)
				continue
			}
			if err != nil {
				log.Error(err, "failed to get terraform instance by ID from API")
				r.UnlockAndRequeue(&terraformInstance, requeueDelay, lockReleased, msg)
				continue
			}
			terraformInstance = *latestTerraformInstance

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if terraformInstance.DeletionScheduled != nil {
					log.Info("terraform instance scheduled for deletion - skipping create")
					break
				}
				customRequeueDelay, err := terraformInstanceCreated(r, &terraformInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile created terraform instance object")
					r.UnlockAndRequeue(
						&terraformInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						&terraformInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				customRequeueDelay, err := terraformInstanceUpdated(r, &terraformInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile updated terraform instance object")
					r.UnlockAndRequeue(
						&terraformInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						&terraformInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				customRequeueDelay, err := terraformInstanceDeleted(r, &terraformInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile deleted terraform instance object")
					r.UnlockAndRequeue(
						&terraformInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("deletion requeued for future reconciliation")
					r.UnlockAndRequeue(
						&terraformInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				deletionTimestamp := util.TimePtr(time.Now().UTC())
				deletedTerraformInstance := v0.TerraformInstance{
					Common: v0.Common{ID: terraformInstance.ID},
					Reconciliation: v0.Reconciliation{
						DeletionAcknowledged: deletionTimestamp,
						DeletionConfirmed:    deletionTimestamp,
						Reconciled:           util.BoolPtr(true),
					},
				}
				if err != nil {
					log.Error(err, "failed to update terraform instance to mark as reconciled")
					r.UnlockAndRequeue(&terraformInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.UpdateTerraformInstance(
					r.APIClient,
					r.APIServer,
					&deletedTerraformInstance,
				)
				if err != nil {
					log.Error(err, "failed to update terraform instance to mark as deleted")
					r.UnlockAndRequeue(&terraformInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.DeleteTerraformInstance(
					r.APIClient,
					r.APIServer,
					*terraformInstance.ID,
				)
				if err != nil {
					log.Error(err, "failed to delete terraform instance")
					r.UnlockAndRequeue(&terraformInstance, requeueDelay, lockReleased, msg)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&terraformInstance,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				reconciledTerraformInstance := v0.TerraformInstance{
					Common:         v0.Common{ID: terraformInstance.ID},
					Reconciliation: v0.Reconciliation{Reconciled: util.BoolPtr(true)},
				}
				updatedTerraformInstance, err := client.UpdateTerraformInstance(
					r.APIClient,
					r.APIServer,
					&reconciledTerraformInstance,
				)
				if err != nil {
					log.Error(err, "failed to update terraform instance to mark as reconciled")
					r.UnlockAndRequeue(&terraformInstance, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"terraform instance marked as reconciled in API",
					"terraform instanceName", updatedTerraformInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&terraformInstance, lockReleased, msg, true); !ok {
				log.Error(errors.New("terraform instance remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("terraform instance unlocked")
			}

			log.Info(fmt.Sprintf(
				"terraform instance successfully reconciled for %s operation",
				notif.Operation,
			))
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
