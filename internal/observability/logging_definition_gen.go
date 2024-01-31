// generated by 'threeport-sdk codegen controller' - do not edit

package observability

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

// LoggingDefinitionReconciler reconciles system state when a LoggingDefinition
// is created, updated or deleted.
func LoggingDefinitionReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("logging definition reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var loggingDefinition v0.LoggingDefinition
			if err := loggingDefinition.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("logging definition reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("loggingDefinitionID", loggingDefinition.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(&loggingDefinition)
			if locked || ok == false {
				r.Requeue(&loggingDefinition, requeueDelay, msg)
				log.V(1).Info("logging definition reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of logging definition")
					r.UnlockAndRequeue(&loggingDefinition, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for logging definition, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&loggingDefinition); !ok {
				r.Requeue(&loggingDefinition, requeueDelay, msg)
				log.V(1).Info("logging definition reconciliation requeued")
				continue
			}

			// retrieve latest version of object
			latestLoggingDefinition, err := client.GetLoggingDefinitionByID(
				r.APIClient,
				r.APIServer,
				*loggingDefinition.ID,
			)
			// check if error is 404 - if object no longer exists, no need to requeue
			if errors.Is(err, client.ErrorObjectNotFound) {
				log.Info(fmt.Sprintf(
					"object with ID %d no longer exists - halting reconciliation",
					*loggingDefinition.ID,
				))
				r.ReleaseLock(&loggingDefinition, lockReleased, msg, true)
				continue
			}
			if err != nil {
				log.Error(err, "failed to get logging definition by ID from API")
				r.UnlockAndRequeue(&loggingDefinition, requeueDelay, lockReleased, msg)
				continue
			}
			loggingDefinition = *latestLoggingDefinition

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if loggingDefinition.DeletionScheduled != nil {
					log.Info("logging definition scheduled for deletion - skipping create")
					break
				}
				customRequeueDelay, err := loggingDefinitionCreated(r, &loggingDefinition, &log)
				if err != nil {
					log.Error(err, "failed to reconcile created logging definition object")
					r.UnlockAndRequeue(
						&loggingDefinition,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						&loggingDefinition,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				customRequeueDelay, err := loggingDefinitionUpdated(r, &loggingDefinition, &log)
				if err != nil {
					log.Error(err, "failed to reconcile updated logging definition object")
					r.UnlockAndRequeue(
						&loggingDefinition,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						&loggingDefinition,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				customRequeueDelay, err := loggingDefinitionDeleted(r, &loggingDefinition, &log)
				if err != nil {
					log.Error(err, "failed to reconcile deleted logging definition object")
					r.UnlockAndRequeue(
						&loggingDefinition,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("deletion requeued for future reconciliation")
					r.UnlockAndRequeue(
						&loggingDefinition,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				deletionTimestamp := util.TimePtr(time.Now().UTC())
				deletedLoggingDefinition := v0.LoggingDefinition{
					Common: v0.Common{ID: loggingDefinition.ID},
					Reconciliation: v0.Reconciliation{
						DeletionAcknowledged: deletionTimestamp,
						DeletionConfirmed:    deletionTimestamp,
						Reconciled:           util.BoolPtr(true),
					},
				}
				if err != nil {
					log.Error(err, "failed to update logging definition to mark as reconciled")
					r.UnlockAndRequeue(&loggingDefinition, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.UpdateLoggingDefinition(
					r.APIClient,
					r.APIServer,
					&deletedLoggingDefinition,
				)
				if err != nil {
					log.Error(err, "failed to update logging definition to mark as deleted")
					r.UnlockAndRequeue(&loggingDefinition, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.DeleteLoggingDefinition(
					r.APIClient,
					r.APIServer,
					*loggingDefinition.ID,
				)
				if err != nil {
					log.Error(err, "failed to delete logging definition")
					r.UnlockAndRequeue(&loggingDefinition, requeueDelay, lockReleased, msg)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&loggingDefinition,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				objectReconciled := true
				reconciledLoggingDefinition := v0.LoggingDefinition{
					Common:         v0.Common{ID: loggingDefinition.ID},
					Reconciliation: v0.Reconciliation{Reconciled: &objectReconciled},
				}
				updatedLoggingDefinition, err := client.UpdateLoggingDefinition(
					r.APIClient,
					r.APIServer,
					&reconciledLoggingDefinition,
				)
				if err != nil {
					log.Error(err, "failed to update logging definition to mark as reconciled")
					r.UnlockAndRequeue(&loggingDefinition, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"logging definition marked as reconciled in API",
					"logging definitionName", updatedLoggingDefinition.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&loggingDefinition, lockReleased, msg, true); !ok {
				log.Error(errors.New("logging definition remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("logging definition unlocked")
			}

			log.Info(fmt.Sprintf(
				"logging definition successfully reconciled for %s operation",
				notif.Operation,
			))
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
