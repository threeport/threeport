// generated by 'threeport-codegen controller' - do not edit

package kubernetesruntime

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

// KubernetesRuntimeInstanceReconciler reconciles system state when a KubernetesRuntimeInstance
// is created, updated or deleted.
func KubernetesRuntimeInstanceReconciler(r *controller.Reconciler) {
	r.ShutdownWait.Add(1)
	reconcilerLog := r.Log.WithValues("reconcilerName", r.Name)
	reconcilerLog.Info("reconciler started")
	shutdown := false

	// Create a channel to receive OS signals
	osSignals := make(chan os.Signal, 1)
	lockReleased := make(chan bool, 1)

	// Register the os signals channel to receive SIGINT and SIGTERM signals
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
					"msgSubject", msg.Subject,
					"msgData", string(msg.Data),
				)
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("kubernetes runtime instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var kubernetesRuntimeInstance v0.KubernetesRuntimeInstance
			if err := kubernetesRuntimeInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("kubernetes runtime instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("kubernetesRuntimeInstanceID", kubernetesRuntimeInstance.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.LastRequeueDelay,
				controller.DefaultInitialRequeueDelay,
				controller.DefaultMaxRequeueDelay,
			)

			// build the notif payload for requeues
			notifPayload, err := kubernetesRuntimeInstance.NotificationPayload(
				notif.Operation,
				true,
				requeueDelay,
			)
			if err != nil {
				log.Error(err, "failed to build notification payload for requeue")
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("kubernetes runtime instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// check for lock on object
			locked, ok := r.CheckLock(&kubernetesRuntimeInstance)
			if locked || ok == false {
				go r.Requeue(&kubernetesRuntimeInstance, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("kubernetes runtime instance reconciliation requeued")
				continue
			}

			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, attempting to unlock and requeue kubernetes runtime instance")
					r.UnlockAndRequeue(&kubernetesRuntimeInstance, msg.Subject, notifPayload, requeueDelay, lockReleased)
					log.V(1).Info("successfully unlocked and requeued kubernetes runtime instance")
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for kubernetes runtime instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&kubernetesRuntimeInstance); !ok {
				go r.Requeue(&kubernetesRuntimeInstance, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("kubernetes runtime instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object if requeued unless object was
			// deleted (in which case we have the latest version)
			if notif.Requeue && notif.Operation != notifications.NotificationOperationDeleted {
				latestKubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
					r.APIClient,
					r.APIServer,
					*kubernetesRuntimeInstance.ID,
				)
				// check if error is 404 - if object no longer exists, no need to requeue
				if errors.Is(err, client.ErrorObjectNotFound) {
					log.Info(fmt.Sprintf(
						"object with ID %d no longer exists - halting reconciliation",
						*kubernetesRuntimeInstance.ID,
					))
					r.ReleaseLock(&kubernetesRuntimeInstance, lockReleased)
					continue
				}
				if err != nil {
					log.Error(err, "failed to get kubernetes runtime instance by ID from API")
					r.UnlockAndRequeue(&kubernetesRuntimeInstance, msg.Subject, notifPayload, requeueDelay, lockReleased)
					continue
				}
				kubernetesRuntimeInstance = *latestKubernetesRuntimeInstance
			}

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if err := kubernetesRuntimeInstanceCreated(r, &kubernetesRuntimeInstance, &log); err != nil {
					log.Error(err, "failed to reconcile created kubernetes runtime instance object")
					r.UnlockAndRequeue(
						&kubernetesRuntimeInstance,
						msg.Subject,
						notifPayload,
						requeueDelay,
						lockReleased,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				if err := kubernetesRuntimeInstanceUpdated(r, &kubernetesRuntimeInstance, &log); err != nil {
					log.Error(err, "failed to reconcile updated kubernetes runtime instance object")
					r.UnlockAndRequeue(
						&kubernetesRuntimeInstance,
						msg.Subject,
						notifPayload,
						requeueDelay,
						lockReleased,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				if err := kubernetesRuntimeInstanceDeleted(r, &kubernetesRuntimeInstance, &log); err != nil {
					log.Error(err, "failed to reconcile deleted kubernetes runtime instance object")
					r.UnlockAndRequeue(
						&kubernetesRuntimeInstance,
						msg.Subject,
						notifPayload,
						requeueDelay,
						lockReleased,
					)
				} else {
					r.ReleaseLock(&kubernetesRuntimeInstance, lockReleased)
					log.Info("kubernetes runtime instance successfully reconciled")
				}
				continue
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&kubernetesRuntimeInstance,
					msg.Subject,
					notifPayload,
					requeueDelay,
					lockReleased,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				objectReconciled := true
				reconciledKubernetesRuntimeInstance := v0.KubernetesRuntimeInstance{
					Common:     v0.Common{ID: kubernetesRuntimeInstance.ID},
					Reconciled: &objectReconciled,
				}
				updatedKubernetesRuntimeInstance, err := client.UpdateKubernetesRuntimeInstance(
					r.APIClient,
					r.APIServer,
					&reconciledKubernetesRuntimeInstance,
				)
				if err != nil {
					log.Error(err, "failed to update kubernetes runtime instance to mark as reconciled")
					r.UnlockAndRequeue(&kubernetesRuntimeInstance, msg.Subject, notifPayload, requeueDelay, lockReleased)
					continue
				}
				log.V(1).Info(
					"kubernetes runtime instance marked as reconciled in API",
					"kubernetes runtime instanceName", updatedKubernetesRuntimeInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&kubernetesRuntimeInstance, lockReleased); !ok {
				log.V(1).Info("kubernetes runtime instance remains locked - will unlock when TTL expires")
			} else {
				log.V(1).Info("kubernetes runtime instance unlocked")
			}

			log.Info("kubernetes runtime instance successfully reconciled")
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
