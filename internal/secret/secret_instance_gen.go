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
	"strings"
	"syscall"
	"time"
)

// SecretInstanceReconciler reconciles system state when a SecretInstance
// is created, updated or deleted.
func SecretInstanceReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("secret instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var secretInstance v0.SecretInstance
			if err := secretInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("secret instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("secretInstanceID", secretInstance.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(&secretInstance)
			if locked || ok == false {
				r.Requeue(&secretInstance, requeueDelay, msg)
				log.V(1).Info("secret instance reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of secret instance")
					r.UnlockAndRequeue(&secretInstance, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for secret instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&secretInstance); !ok {
				r.Requeue(&secretInstance, requeueDelay, msg)
				log.V(1).Info("secret instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object
			latestSecretInstance, err := client.GetSecretInstanceByID(
				r.APIClient,
				r.APIServer,
				*secretInstance.ID,
			)
			// check if error is 404 - if object no longer exists, no need to requeue
			if errors.Is(err, client.ErrObjectNotFound) {
				log.Info(fmt.Sprintf(
					"object with ID %d no longer exists - halting reconciliation",
					*secretInstance.ID,
				))
				r.ReleaseLock(&secretInstance, lockReleased, msg, true)
				continue
			}
			if err != nil {
				log.Error(err, "failed to get secret instance by ID from API")
				r.UnlockAndRequeue(&secretInstance, requeueDelay, lockReleased, msg)
				continue
			}
			secretInstance = *latestSecretInstance

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if secretInstance.DeletionScheduled != nil {
					log.Info("secret instance scheduled for deletion - skipping create")
					break
				}
				customRequeueDelay, err := secretInstanceCreated(r, &secretInstance, &log)
				if err != nil {
					errorMsg := "failed to reconcile created secret instance object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("SecretInstanceNotCreated"),
							Type:   util.Ptr("Normal"),
						},
						secretInstance.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&secretInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						&secretInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				customRequeueDelay, err := secretInstanceUpdated(r, &secretInstance, &log)
				if err != nil {
					errorMsg := "failed to reconcile updated secret instance object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("SecretInstanceNotUpdated"),
							Type:   util.Ptr("Normal"),
						},
						secretInstance.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&secretInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						&secretInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				customRequeueDelay, err := secretInstanceDeleted(r, &secretInstance, &log)
				if err != nil {
					errorMsg := "failed to reconcile deleted secret instance object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("SecretInstanceNotUpdated"),
							Type:   util.Ptr("Normal"),
						},
						secretInstance.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&secretInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("deletion requeued for future reconciliation")
					r.UnlockAndRequeue(
						&secretInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				deletionTimestamp := util.Ptr(time.Now().UTC())
				deletedSecretInstance := v0.SecretInstance{
					Common: v0.Common{ID: secretInstance.ID},
					Reconciliation: v0.Reconciliation{
						DeletionAcknowledged: deletionTimestamp,
						DeletionConfirmed:    deletionTimestamp,
						Reconciled:           util.Ptr(true),
					},
				}
				if err != nil {
					log.Error(err, "failed to update secret instance to mark as reconciled")
					r.UnlockAndRequeue(&secretInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.UpdateSecretInstance(
					r.APIClient,
					r.APIServer,
					&deletedSecretInstance,
				)
				if err != nil {
					log.Error(err, "failed to update secret instance to mark as deleted")
					r.UnlockAndRequeue(&secretInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.DeleteSecretInstance(
					r.APIClient,
					r.APIServer,
					*secretInstance.ID,
				)
				if err != nil {
					log.Error(err, "failed to delete secret instance")
					r.UnlockAndRequeue(&secretInstance, requeueDelay, lockReleased, msg)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&secretInstance,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				reconciledSecretInstance := v0.SecretInstance{
					Common:         v0.Common{ID: secretInstance.ID},
					Reconciliation: v0.Reconciliation{Reconciled: util.Ptr(true)},
				}
				updatedSecretInstance, err := client.UpdateSecretInstance(
					r.APIClient,
					r.APIServer,
					&reconciledSecretInstance,
				)
				if err != nil {
					log.Error(err, "failed to update secret instance to mark as reconciled")
					r.UnlockAndRequeue(&secretInstance, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"secret instance marked as reconciled in API",
					"secret instanceName", updatedSecretInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&secretInstance, lockReleased, msg, true); !ok {
				log.Error(errors.New("secret instance remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("secret instance unlocked")
			}

			successMsg := fmt.Sprintf(
				"secret instance successfully reconciled for %s operation",
				strings.ToLower(string(notif.Operation)),
			)
			if err := r.EventsRecorder.RecordEvent(
				&v0.Event{
					Note:   util.Ptr(successMsg),
					Reason: util.Ptr("SecretInstanceSuccessfullyReconciled"),
					Type:   util.Ptr("Normal"),
				},
				secretInstance.ID,
			); err != nil {
				log.Error(err, "failed to record event for successful secret instance reconciliation")
			}
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
