// generated by 'threeport-sdk gen' for controller scaffolding - do not edit

package aws

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

// AwsRelationalDatabaseInstanceReconciler reconciles system state when a AwsRelationalDatabaseInstance
// is created, updated or deleted.
func AwsRelationalDatabaseInstanceReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("aws relational database instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var awsRelationalDatabaseInstance v0.AwsRelationalDatabaseInstance
			if err := awsRelationalDatabaseInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("aws relational database instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("awsRelationalDatabaseInstanceID", awsRelationalDatabaseInstance.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(&awsRelationalDatabaseInstance)
			if locked || ok == false {
				r.Requeue(&awsRelationalDatabaseInstance, requeueDelay, msg)
				log.V(1).Info("aws relational database instance reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of aws relational database instance")
					r.UnlockAndRequeue(&awsRelationalDatabaseInstance, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for aws relational database instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&awsRelationalDatabaseInstance); !ok {
				r.Requeue(&awsRelationalDatabaseInstance, requeueDelay, msg)
				log.V(1).Info("aws relational database instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object
			latestAwsRelationalDatabaseInstance, err := client.GetAwsRelationalDatabaseInstanceByID(
				r.APIClient,
				r.APIServer,
				*awsRelationalDatabaseInstance.ID,
			)
			// check if error is 404 - if object no longer exists, no need to requeue
			if errors.Is(err, client.ErrObjectNotFound) {
				log.Info(fmt.Sprintf(
					"object with ID %d no longer exists - halting reconciliation",
					*awsRelationalDatabaseInstance.ID,
				))
				r.ReleaseLock(&awsRelationalDatabaseInstance, lockReleased, msg, true)
				continue
			}
			if err != nil {
				log.Error(err, "failed to get aws relational database instance by ID from API")
				r.UnlockAndRequeue(&awsRelationalDatabaseInstance, requeueDelay, lockReleased, msg)
				continue
			}
			awsRelationalDatabaseInstance = *latestAwsRelationalDatabaseInstance

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if awsRelationalDatabaseInstance.DeletionScheduled != nil {
					log.Info("aws relational database instance scheduled for deletion - skipping create")
					break
				}
				customRequeueDelay, err := awsRelationalDatabaseInstanceCreated(r, &awsRelationalDatabaseInstance, &log)
				if err != nil {
					errorMsg := "failed to reconcile created aws relational database instance object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("AwsRelationalDatabaseInstanceNotCreated"),
							Type:   util.Ptr("Normal"),
						},
						awsRelationalDatabaseInstance.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&awsRelationalDatabaseInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						&awsRelationalDatabaseInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				customRequeueDelay, err := awsRelationalDatabaseInstanceUpdated(r, &awsRelationalDatabaseInstance, &log)
				if err != nil {
					errorMsg := "failed to reconcile updated aws relational database instance object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("AwsRelationalDatabaseInstanceNotUpdated"),
							Type:   util.Ptr("Normal"),
						},
						awsRelationalDatabaseInstance.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&awsRelationalDatabaseInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						&awsRelationalDatabaseInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				customRequeueDelay, err := awsRelationalDatabaseInstanceDeleted(r, &awsRelationalDatabaseInstance, &log)
				if err != nil {
					errorMsg := "failed to reconcile deleted aws relational database instance object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("AwsRelationalDatabaseInstanceNotUpdated"),
							Type:   util.Ptr("Normal"),
						},
						awsRelationalDatabaseInstance.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&awsRelationalDatabaseInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("deletion requeued for future reconciliation")
					r.UnlockAndRequeue(
						&awsRelationalDatabaseInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				deletionTimestamp := util.Ptr(time.Now().UTC())
				deletedAwsRelationalDatabaseInstance := v0.AwsRelationalDatabaseInstance{
					Common: v0.Common{ID: awsRelationalDatabaseInstance.ID},
					Reconciliation: v0.Reconciliation{
						DeletionAcknowledged: deletionTimestamp,
						DeletionConfirmed:    deletionTimestamp,
						Reconciled:           util.Ptr(true),
					},
				}
				if err != nil {
					log.Error(err, "failed to update aws relational database instance to mark as reconciled")
					r.UnlockAndRequeue(&awsRelationalDatabaseInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.UpdateAwsRelationalDatabaseInstance(
					r.APIClient,
					r.APIServer,
					&deletedAwsRelationalDatabaseInstance,
				)
				if err != nil {
					log.Error(err, "failed to update aws relational database instance to mark as deleted")
					r.UnlockAndRequeue(&awsRelationalDatabaseInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.DeleteAwsRelationalDatabaseInstance(
					r.APIClient,
					r.APIServer,
					*awsRelationalDatabaseInstance.ID,
				)
				if err != nil {
					log.Error(err, "failed to delete aws relational database instance")
					r.UnlockAndRequeue(&awsRelationalDatabaseInstance, requeueDelay, lockReleased, msg)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&awsRelationalDatabaseInstance,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				reconciledAwsRelationalDatabaseInstance := v0.AwsRelationalDatabaseInstance{
					Common:         v0.Common{ID: awsRelationalDatabaseInstance.ID},
					Reconciliation: v0.Reconciliation{Reconciled: util.Ptr(true)},
				}
				updatedAwsRelationalDatabaseInstance, err := client.UpdateAwsRelationalDatabaseInstance(
					r.APIClient,
					r.APIServer,
					&reconciledAwsRelationalDatabaseInstance,
				)
				if err != nil {
					log.Error(err, "failed to update aws relational database instance to mark as reconciled")
					r.UnlockAndRequeue(&awsRelationalDatabaseInstance, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"aws relational database instance marked as reconciled in API",
					"aws relational database instanceName", updatedAwsRelationalDatabaseInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&awsRelationalDatabaseInstance, lockReleased, msg, true); !ok {
				log.Error(errors.New("aws relational database instance remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("aws relational database instance unlocked")
			}

			successMsg := fmt.Sprintf(
				"aws relational database instance successfully reconciled for %s operation",
				strings.ToLower(string(notif.Operation)),
			)
			if err := r.EventsRecorder.RecordEvent(
				&v0.Event{
					Note:   util.Ptr(successMsg),
					Reason: util.Ptr("AwsRelationalDatabaseInstanceSuccessfullyReconciled"),
					Type:   util.Ptr("Normal"),
				},
				awsRelationalDatabaseInstance.ID,
			); err != nil {
				log.Error(err, "failed to record event for successful aws relational database instance reconciliation")
			}
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
