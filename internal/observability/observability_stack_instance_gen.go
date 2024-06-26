// generated by 'threeport-sdk gen' for controller scaffolding - do not edit

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

// ObservabilityStackInstanceReconciler reconciles system state when a ObservabilityStackInstance
// is created, updated or deleted.
func ObservabilityStackInstanceReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("observability stack instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var observabilityStackInstance v0.ObservabilityStackInstance
			if err := observabilityStackInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("observability stack instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("observabilityStackInstanceID", observabilityStackInstance.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(&observabilityStackInstance)
			if locked || ok == false {
				r.Requeue(&observabilityStackInstance, requeueDelay, msg)
				log.V(1).Info("observability stack instance reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of observability stack instance")
					r.UnlockAndRequeue(&observabilityStackInstance, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for observability stack instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&observabilityStackInstance); !ok {
				r.Requeue(&observabilityStackInstance, requeueDelay, msg)
				log.V(1).Info("observability stack instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object
			latestObservabilityStackInstance, err := client.GetObservabilityStackInstanceByID(
				r.APIClient,
				r.APIServer,
				*observabilityStackInstance.ID,
			)
			// check if error is 404 - if object no longer exists, no need to requeue
			if errors.Is(err, client.ErrObjectNotFound) {
				log.Info(fmt.Sprintf(
					"object with ID %d no longer exists - halting reconciliation",
					*observabilityStackInstance.ID,
				))
				r.ReleaseLock(&observabilityStackInstance, lockReleased, msg, true)
				continue
			}
			if err != nil {
				log.Error(err, "failed to get observability stack instance by ID from API")
				r.UnlockAndRequeue(&observabilityStackInstance, requeueDelay, lockReleased, msg)
				continue
			}
			observabilityStackInstance = *latestObservabilityStackInstance

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if observabilityStackInstance.DeletionScheduled != nil {
					log.Info("observability stack instance scheduled for deletion - skipping create")
					break
				}
				customRequeueDelay, err := observabilityStackInstanceCreated(r, &observabilityStackInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile created observability stack instance object")
					r.UnlockAndRequeue(
						&observabilityStackInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						&observabilityStackInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				customRequeueDelay, err := observabilityStackInstanceUpdated(r, &observabilityStackInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile updated observability stack instance object")
					r.UnlockAndRequeue(
						&observabilityStackInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						&observabilityStackInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				customRequeueDelay, err := observabilityStackInstanceDeleted(r, &observabilityStackInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile deleted observability stack instance object")
					r.UnlockAndRequeue(
						&observabilityStackInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("deletion requeued for future reconciliation")
					r.UnlockAndRequeue(
						&observabilityStackInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				deletionTimestamp := util.TimePtr(time.Now().UTC())
				deletedObservabilityStackInstance := v0.ObservabilityStackInstance{
					Common: v0.Common{ID: observabilityStackInstance.ID},
					Reconciliation: v0.Reconciliation{
						DeletionAcknowledged: deletionTimestamp,
						DeletionConfirmed:    deletionTimestamp,
						Reconciled:           util.BoolPtr(true),
					},
				}
				if err != nil {
					log.Error(err, "failed to update observability stack instance to mark as reconciled")
					r.UnlockAndRequeue(&observabilityStackInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.UpdateObservabilityStackInstance(
					r.APIClient,
					r.APIServer,
					&deletedObservabilityStackInstance,
				)
				if err != nil {
					log.Error(err, "failed to update observability stack instance to mark as deleted")
					r.UnlockAndRequeue(&observabilityStackInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.DeleteObservabilityStackInstance(
					r.APIClient,
					r.APIServer,
					*observabilityStackInstance.ID,
				)
				if err != nil {
					log.Error(err, "failed to delete observability stack instance")
					r.UnlockAndRequeue(&observabilityStackInstance, requeueDelay, lockReleased, msg)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&observabilityStackInstance,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				reconciledObservabilityStackInstance := v0.ObservabilityStackInstance{
					Common:         v0.Common{ID: observabilityStackInstance.ID},
					Reconciliation: v0.Reconciliation{Reconciled: util.BoolPtr(true)},
				}
				updatedObservabilityStackInstance, err := client.UpdateObservabilityStackInstance(
					r.APIClient,
					r.APIServer,
					&reconciledObservabilityStackInstance,
				)
				if err != nil {
					log.Error(err, "failed to update observability stack instance to mark as reconciled")
					r.UnlockAndRequeue(&observabilityStackInstance, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"observability stack instance marked as reconciled in API",
					"observability stack instanceName", updatedObservabilityStackInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&observabilityStackInstance, lockReleased, msg, true); !ok {
				log.Error(errors.New("observability stack instance remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("observability stack instance unlocked")
			}

			log.Info(fmt.Sprintf(
				"observability stack instance successfully reconciled for %s operation",
				notif.Operation,
			))
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
