// generated by 'threeport-sdk codegen controller' - do not edit

package controlplane

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

// ControlPlaneInstanceReconciler reconciles system state when a ControlPlaneInstance
// is created, updated or deleted.
func ControlPlaneInstanceReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("control plane instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var controlPlaneInstance v0.ControlPlaneInstance
			if err := controlPlaneInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("control plane instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("controlPlaneInstanceID", controlPlaneInstance.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(&controlPlaneInstance)
			if locked || ok == false {
				r.Requeue(&controlPlaneInstance, requeueDelay, msg)
				log.V(1).Info("control plane instance reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of control plane instance")
					r.UnlockAndRequeue(&controlPlaneInstance, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for control plane instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&controlPlaneInstance); !ok {
				r.Requeue(&controlPlaneInstance, requeueDelay, msg)
				log.V(1).Info("control plane instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object
			latestControlPlaneInstance, err := client.GetControlPlaneInstanceByID(
				r.APIClient,
				r.APIServer,
				*controlPlaneInstance.ID,
			)
			// check if error is 404 - if object no longer exists, no need to requeue
			if errors.Is(err, client.ErrObjectNotFound) {
				log.Info(fmt.Sprintf(
					"object with ID %d no longer exists - halting reconciliation",
					*controlPlaneInstance.ID,
				))
				r.ReleaseLock(&controlPlaneInstance, lockReleased, msg, true)
				continue
			}
			if err != nil {
				log.Error(err, "failed to get control plane instance by ID from API")
				r.UnlockAndRequeue(&controlPlaneInstance, requeueDelay, lockReleased, msg)
				continue
			}
			controlPlaneInstance = *latestControlPlaneInstance

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if controlPlaneInstance.DeletionScheduled != nil {
					log.Info("control plane instance scheduled for deletion - skipping create")
					break
				}
				customRequeueDelay, err := controlPlaneInstanceCreated(r, &controlPlaneInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile created control plane instance object")
					r.UnlockAndRequeue(
						&controlPlaneInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						&controlPlaneInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				customRequeueDelay, err := controlPlaneInstanceUpdated(r, &controlPlaneInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile updated control plane instance object")
					r.UnlockAndRequeue(
						&controlPlaneInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						&controlPlaneInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				customRequeueDelay, err := controlPlaneInstanceDeleted(r, &controlPlaneInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile deleted control plane instance object")
					r.UnlockAndRequeue(
						&controlPlaneInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("deletion requeued for future reconciliation")
					r.UnlockAndRequeue(
						&controlPlaneInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				deletionTimestamp := util.TimePtr(time.Now().UTC())
				deletedControlPlaneInstance := v0.ControlPlaneInstance{
					Common: v0.Common{ID: controlPlaneInstance.ID},
					Reconciliation: v0.Reconciliation{
						DeletionAcknowledged: deletionTimestamp,
						DeletionConfirmed:    deletionTimestamp,
						Reconciled:           util.BoolPtr(true),
					},
				}
				if err != nil {
					log.Error(err, "failed to update control plane instance to mark as reconciled")
					r.UnlockAndRequeue(&controlPlaneInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.UpdateControlPlaneInstance(
					r.APIClient,
					r.APIServer,
					&deletedControlPlaneInstance,
				)
				if err != nil {
					log.Error(err, "failed to update control plane instance to mark as deleted")
					r.UnlockAndRequeue(&controlPlaneInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.DeleteControlPlaneInstance(
					r.APIClient,
					r.APIServer,
					*controlPlaneInstance.ID,
				)
				if err != nil {
					log.Error(err, "failed to delete control plane instance")
					r.UnlockAndRequeue(&controlPlaneInstance, requeueDelay, lockReleased, msg)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&controlPlaneInstance,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				reconciledControlPlaneInstance := v0.ControlPlaneInstance{
					Common:         v0.Common{ID: controlPlaneInstance.ID},
					Reconciliation: v0.Reconciliation{Reconciled: util.BoolPtr(true)},
				}
				updatedControlPlaneInstance, err := client.UpdateControlPlaneInstance(
					r.APIClient,
					r.APIServer,
					&reconciledControlPlaneInstance,
				)
				if err != nil {
					log.Error(err, "failed to update control plane instance to mark as reconciled")
					r.UnlockAndRequeue(&controlPlaneInstance, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"control plane instance marked as reconciled in API",
					"control plane instanceName", updatedControlPlaneInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&controlPlaneInstance, lockReleased, msg, true); !ok {
				log.Error(errors.New("control plane instance remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("control plane instance unlocked")
			}

			log.Info(fmt.Sprintf(
				"control plane instance successfully reconciled for %s operation",
				notif.Operation,
			))
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
