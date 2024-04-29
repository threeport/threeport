// generated by 'threeport-sdk gen' for controller scaffolding - do not edit

package secret

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

// SecretDefinitionReconciler reconciles system state when a SecretDefinition
// is created, updated or deleted.
func SecretDefinitionReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("secret definition reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var secretDefinition v0.SecretDefinition
			if err := secretDefinition.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("secret definition reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("secretDefinitionID", secretDefinition.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(&secretDefinition)
			if locked || ok == false {
				r.Requeue(&secretDefinition, requeueDelay, msg)
				log.V(1).Info("secret definition reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of secret definition")
					r.UnlockAndRequeue(&secretDefinition, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for secret definition, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&secretDefinition); !ok {
				r.Requeue(&secretDefinition, requeueDelay, msg)
				log.V(1).Info("secret definition reconciliation requeued")
				continue
			}

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if secretDefinition.DeletionScheduled != nil {
					log.Info("secret definition scheduled for deletion - skipping create")
					break
				}
				customRequeueDelay, err := secretDefinitionCreated(r, &secretDefinition, &log)
				if err != nil {
					errorMsg := "failed to reconcile created secret definition object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("SecretDefinitionNotCreated"),
							Type:   util.Ptr("Normal"),
						},
						secretDefinition.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&secretDefinition,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						&secretDefinition,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				customRequeueDelay, err := secretDefinitionUpdated(r, &secretDefinition, &log)
				if err != nil {
					errorMsg := "failed to reconcile updated secret definition object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("SecretDefinitionNotUpdated"),
							Type:   util.Ptr("Normal"),
						},
						secretDefinition.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&secretDefinition,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						&secretDefinition,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				customRequeueDelay, err := secretDefinitionDeleted(r, &secretDefinition, &log)
				if err != nil {
					errorMsg := "failed to reconcile deleted secret definition object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("SecretDefinitionNotUpdated"),
							Type:   util.Ptr("Normal"),
						},
						secretDefinition.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&secretDefinition,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("deletion requeued for future reconciliation")
					r.UnlockAndRequeue(
						&secretDefinition,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				deletionTimestamp := util.TimePtr(time.Now().UTC())
				deletedSecretDefinition := v0.SecretDefinition{
					Common: v0.Common{ID: secretDefinition.ID},
					Reconciliation: v0.Reconciliation{
						DeletionAcknowledged: deletionTimestamp,
						DeletionConfirmed:    deletionTimestamp,
						Reconciled:           util.BoolPtr(true),
					},
				}
				if err != nil {
					log.Error(err, "failed to update secret definition to mark as reconciled")
					r.UnlockAndRequeue(&secretDefinition, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.UpdateSecretDefinition(
					r.APIClient,
					r.APIServer,
					&deletedSecretDefinition,
				)
				if err != nil {
					log.Error(err, "failed to update secret definition to mark as deleted")
					r.UnlockAndRequeue(&secretDefinition, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.DeleteSecretDefinition(
					r.APIClient,
					r.APIServer,
					*secretDefinition.ID,
				)
				if err != nil {
					log.Error(err, "failed to delete secret definition")
					r.UnlockAndRequeue(&secretDefinition, requeueDelay, lockReleased, msg)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&secretDefinition,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				reconciledSecretDefinition := v0.SecretDefinition{
					Common:         v0.Common{ID: secretDefinition.ID},
					Reconciliation: v0.Reconciliation{Reconciled: util.BoolPtr(true)},
				}
				updatedSecretDefinition, err := client.UpdateSecretDefinition(
					r.APIClient,
					r.APIServer,
					&reconciledSecretDefinition,
				)
				if err != nil {
					log.Error(err, "failed to update secret definition to mark as reconciled")
					r.UnlockAndRequeue(&secretDefinition, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"secret definition marked as reconciled in API",
					"secret definitionName", updatedSecretDefinition.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&secretDefinition, lockReleased, msg, true); !ok {
				log.Error(errors.New("secret definition remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("secret definition unlocked")
			}

			successMsg := "secret definition successfully reconciled for %s operation"
			log.Info(fmt.Sprintf(
				successMsg,
				notif.Operation,
			))
			if err := r.EventsRecorder.RecordEvent(
				&v0.Event{
					Note:   util.Ptr(successMsg),
					Reason: util.Ptr("SecretDefinitionSuccessfullyReconciled"),
					Type:   util.Ptr("Normal"),
				},
				secretDefinition.ID,
			); err != nil {
				log.Error(err, "failed to record event for successful secret definition reconciliation")
			}
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
