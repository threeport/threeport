// generated by 'threeport-sdk gen' for controller scaffolding - do not edit

package gateway

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

// GatewayInstanceReconciler reconciles system state when a GatewayInstance
// is created, updated or deleted.
func GatewayInstanceReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("gateway instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var gatewayInstance v0.GatewayInstance
			if err := gatewayInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("gateway instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("gatewayInstanceID", gatewayInstance.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(&gatewayInstance)
			if locked || ok == false {
				r.Requeue(&gatewayInstance, requeueDelay, msg)
				log.V(1).Info("gateway instance reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of gateway instance")
					r.UnlockAndRequeue(&gatewayInstance, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for gateway instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&gatewayInstance); !ok {
				r.Requeue(&gatewayInstance, requeueDelay, msg)
				log.V(1).Info("gateway instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object
			latestGatewayInstance, err := client.GetGatewayInstanceByID(
				r.APIClient,
				r.APIServer,
				*gatewayInstance.ID,
			)
			// check if error is 404 - if object no longer exists, no need to requeue
			if errors.Is(err, client.ErrObjectNotFound) {
				log.Info(fmt.Sprintf(
					"object with ID %d no longer exists - halting reconciliation",
					*gatewayInstance.ID,
				))
				r.ReleaseLock(&gatewayInstance, lockReleased, msg, true)
				continue
			}
			if err != nil {
				log.Error(err, "failed to get gateway instance by ID from API")
				r.UnlockAndRequeue(&gatewayInstance, requeueDelay, lockReleased, msg)
				continue
			}
			gatewayInstance = *latestGatewayInstance

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if gatewayInstance.DeletionScheduled != nil {
					log.Info("gateway instance scheduled for deletion - skipping create")
					break
				}
				customRequeueDelay, err := gatewayInstanceCreated(r, &gatewayInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile created gateway instance object")
					r.UnlockAndRequeue(
						&gatewayInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						&gatewayInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				customRequeueDelay, err := gatewayInstanceUpdated(r, &gatewayInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile updated gateway instance object")
					r.UnlockAndRequeue(
						&gatewayInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						&gatewayInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				customRequeueDelay, err := gatewayInstanceDeleted(r, &gatewayInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile deleted gateway instance object")
					r.UnlockAndRequeue(
						&gatewayInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("deletion requeued for future reconciliation")
					r.UnlockAndRequeue(
						&gatewayInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				deletionTimestamp := util.TimePtr(time.Now().UTC())
				deletedGatewayInstance := v0.GatewayInstance{
					Common: v0.Common{ID: gatewayInstance.ID},
					Reconciliation: v0.Reconciliation{
						DeletionAcknowledged: deletionTimestamp,
						DeletionConfirmed:    deletionTimestamp,
						Reconciled:           util.BoolPtr(true),
					},
				}
				if err != nil {
					log.Error(err, "failed to update gateway instance to mark as reconciled")
					r.UnlockAndRequeue(&gatewayInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.UpdateGatewayInstance(
					r.APIClient,
					r.APIServer,
					&deletedGatewayInstance,
				)
				if err != nil {
					log.Error(err, "failed to update gateway instance to mark as deleted")
					r.UnlockAndRequeue(&gatewayInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.DeleteGatewayInstance(
					r.APIClient,
					r.APIServer,
					*gatewayInstance.ID,
				)
				if err != nil {
					log.Error(err, "failed to delete gateway instance")
					r.UnlockAndRequeue(&gatewayInstance, requeueDelay, lockReleased, msg)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&gatewayInstance,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				reconciledGatewayInstance := v0.GatewayInstance{
					Common:         v0.Common{ID: gatewayInstance.ID},
					Reconciliation: v0.Reconciliation{Reconciled: util.BoolPtr(true)},
				}
				updatedGatewayInstance, err := client.UpdateGatewayInstance(
					r.APIClient,
					r.APIServer,
					&reconciledGatewayInstance,
				)
				if err != nil {
					log.Error(err, "failed to update gateway instance to mark as reconciled")
					r.UnlockAndRequeue(&gatewayInstance, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"gateway instance marked as reconciled in API",
					"gateway instanceName", updatedGatewayInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&gatewayInstance, lockReleased, msg, true); !ok {
				log.Error(errors.New("gateway instance remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("gateway instance unlocked")
			}

			log.Info(fmt.Sprintf(
				"gateway instance successfully reconciled for %s operation",
				notif.Operation,
			))
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
