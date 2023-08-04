// generated by 'threeport-codegen controller' - do not edit

package kubernetesruntime

import (
	"errors"
	"fmt"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	"os"
	"os/signal"
	"syscall"
)

// KubernetesRuntimeDefinitionReconciler reconciles system state when a KubernetesRuntimeDefinition
// is created, updated or deleted.
func KubernetesRuntimeDefinitionReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("kubernetes runtime definition reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var kubernetesRuntimeDefinition v0.KubernetesRuntimeDefinition
			if err := kubernetesRuntimeDefinition.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("kubernetes runtime definition reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("kubernetesRuntimeDefinitionID", kubernetesRuntimeDefinition.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.LastRequeueDelay,
				controller.DefaultInitialRequeueDelay,
				controller.DefaultMaxRequeueDelay,
			)

			// build the notif payload for requeues
			notifPayload, err := kubernetesRuntimeDefinition.NotificationPayload(
				notif.Operation,
				true,
				requeueDelay,
			)
			if err != nil {
				log.Error(err, "failed to build notification payload for requeue")
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("kubernetes runtime definition reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// check for lock on object
			locked, ok := r.CheckLock(&kubernetesRuntimeDefinition)
			if locked || ok == false {
				go r.Requeue(&kubernetesRuntimeDefinition, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("kubernetes runtime definition reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of kubernetes runtime definition")
					r.UnlockAndRequeue(&kubernetesRuntimeDefinition, msg.Subject, notifPayload, requeueDelay, lockReleased)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for kubernetes runtime definition, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&kubernetesRuntimeDefinition); !ok {
				go r.Requeue(&kubernetesRuntimeDefinition, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("kubernetes runtime definition reconciliation requeued")
				continue
			}

			// retrieve latest version of object if requeued unless object was
			// deleted (in which case we have the latest version)
			if notif.Requeue && notif.Operation != notifications.NotificationOperationDeleted {
				latestKubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
					r.APIClient,
					r.APIServer,
					*kubernetesRuntimeDefinition.ID,
				)
				// check if error is 404 - if object no longer exists, no need to requeue
				if errors.Is(err, client.ErrorObjectNotFound) {
					log.Info(fmt.Sprintf(
						"object with ID %d no longer exists - halting reconciliation",
						*kubernetesRuntimeDefinition.ID,
					))
					r.ReleaseLock(&kubernetesRuntimeDefinition, lockReleased)
					continue
				}
				if err != nil {
					log.Error(err, "failed to get kubernetes runtime definition by ID from API")
					r.UnlockAndRequeue(&kubernetesRuntimeDefinition, msg.Subject, notifPayload, requeueDelay, lockReleased)
					continue
				}
				kubernetesRuntimeDefinition = *latestKubernetesRuntimeDefinition
			}

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if err := kubernetesRuntimeDefinitionCreated(r, &kubernetesRuntimeDefinition, &log); err != nil {
					log.Error(err, "failed to reconcile created kubernetes runtime definition object")
					r.UnlockAndRequeue(
						&kubernetesRuntimeDefinition,
						msg.Subject,
						notifPayload,
						requeueDelay,
						lockReleased,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				if err := kubernetesRuntimeDefinitionUpdated(r, &kubernetesRuntimeDefinition, &log); err != nil {
					log.Error(err, "failed to reconcile updated kubernetes runtime definition object")
					r.UnlockAndRequeue(
						&kubernetesRuntimeDefinition,
						msg.Subject,
						notifPayload,
						requeueDelay,
						lockReleased,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				if err := kubernetesRuntimeDefinitionDeleted(r, &kubernetesRuntimeDefinition, &log); err != nil {
					log.Error(err, "failed to reconcile deleted kubernetes runtime definition object")
					r.UnlockAndRequeue(
						&kubernetesRuntimeDefinition,
						msg.Subject,
						notifPayload,
						requeueDelay,
						lockReleased,
					)
				} else {
					r.ReleaseLock(&kubernetesRuntimeDefinition, lockReleased)
					log.Info("kubernetes runtime definition successfully reconciled")
				}
				continue
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&kubernetesRuntimeDefinition,
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
				reconciledKubernetesRuntimeDefinition := v0.KubernetesRuntimeDefinition{
					Common:     v0.Common{ID: kubernetesRuntimeDefinition.ID},
					Reconciled: &objectReconciled,
				}
				updatedKubernetesRuntimeDefinition, err := client.UpdateKubernetesRuntimeDefinition(
					r.APIClient,
					r.APIServer,
					&reconciledKubernetesRuntimeDefinition,
				)
				if err != nil {
					log.Error(err, "failed to update kubernetes runtime definition to mark as reconciled")
					r.UnlockAndRequeue(&kubernetesRuntimeDefinition, msg.Subject, notifPayload, requeueDelay, lockReleased)
					continue
				}
				log.V(1).Info(
					"kubernetes runtime definition marked as reconciled in API",
					"kubernetes runtime definitionName", updatedKubernetesRuntimeDefinition.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&kubernetesRuntimeDefinition, lockReleased); !ok {
				log.V(1).Info("kubernetes runtime definition remains locked - will unlock when TTL expires")
			} else {
				log.V(1).Info("kubernetes runtime definition unlocked")
			}

			log.Info("kubernetes runtime definition successfully reconciled")
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
