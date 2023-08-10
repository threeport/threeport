// generated by 'threeport-codegen controller' - do not edit

package aws

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

// AwsEksKubernetesRuntimeInstanceReconciler reconciles system state when a AwsEksKubernetesRuntimeInstance
// is created, updated or deleted.
func AwsEksKubernetesRuntimeInstanceReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("aws eks kubernetes runtime instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var awsEksKubernetesRuntimeInstance v0.AwsEksKubernetesRuntimeInstance
			if err := awsEksKubernetesRuntimeInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("aws eks kubernetes runtime instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("awsEksKubernetesRuntimeInstanceID", awsEksKubernetesRuntimeInstance.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(&awsEksKubernetesRuntimeInstance)
			if locked || ok == false {
				r.Requeue(&awsEksKubernetesRuntimeInstance, requeueDelay, msg)
				log.V(1).Info("aws eks kubernetes runtime instance reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of aws eks kubernetes runtime instance")
					r.UnlockAndRequeue(&awsEksKubernetesRuntimeInstance, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for aws eks kubernetes runtime instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&awsEksKubernetesRuntimeInstance); !ok {
				r.Requeue(&awsEksKubernetesRuntimeInstance, requeueDelay, msg)
				log.V(1).Info("aws eks kubernetes runtime instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object unless object was
			// deleted (in which case we have the latest version)
			if notif.Operation != notifications.NotificationOperationDeleted {
				latestAwsEksKubernetesRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByID(
					r.APIClient,
					r.APIServer,
					*awsEksKubernetesRuntimeInstance.ID,
				)
				// check if error is 404 - if object no longer exists, no need to requeue
				if errors.Is(err, client.ErrorObjectNotFound) {
					log.Info(fmt.Sprintf(
						"object with ID %d no longer exists - halting reconciliation",
						*awsEksKubernetesRuntimeInstance.ID,
					))
					r.ReleaseLock(&awsEksKubernetesRuntimeInstance, lockReleased, msg, true)
					continue
				}
				if err != nil {
					log.Error(err, "failed to get aws eks kubernetes runtime instance by ID from API")
					r.UnlockAndRequeue(&awsEksKubernetesRuntimeInstance, requeueDelay, lockReleased, msg)
					continue
				}
				awsEksKubernetesRuntimeInstance = *latestAwsEksKubernetesRuntimeInstance
			}

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if err := awsEksKubernetesRuntimeInstanceCreated(r, &awsEksKubernetesRuntimeInstance, &log); err != nil {
					log.Error(err, "failed to reconcile created aws eks kubernetes runtime instance object")
					r.UnlockAndRequeue(
						&awsEksKubernetesRuntimeInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				if err := awsEksKubernetesRuntimeInstanceUpdated(r, &awsEksKubernetesRuntimeInstance, &log); err != nil {
					log.Error(err, "failed to reconcile updated aws eks kubernetes runtime instance object")
					r.UnlockAndRequeue(
						&awsEksKubernetesRuntimeInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				if err := awsEksKubernetesRuntimeInstanceDeleted(r, &awsEksKubernetesRuntimeInstance, &log); err != nil {
					log.Error(err, "failed to reconcile deleted aws eks kubernetes runtime instance object")
					r.UnlockAndRequeue(
						&awsEksKubernetesRuntimeInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
				} else {
					r.ReleaseLock(&awsEksKubernetesRuntimeInstance, lockReleased, msg, true)
					log.Info("aws eks kubernetes runtime instance successfully reconciled")
					msg.Ack()
				}
				continue
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&awsEksKubernetesRuntimeInstance,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				objectReconciled := true
				reconciledAwsEksKubernetesRuntimeInstance := v0.AwsEksKubernetesRuntimeInstance{
					Common:     v0.Common{ID: awsEksKubernetesRuntimeInstance.ID},
					Reconciled: &objectReconciled,
				}
				updatedAwsEksKubernetesRuntimeInstance, err := client.UpdateAwsEksKubernetesRuntimeInstance(
					r.APIClient,
					r.APIServer,
					&reconciledAwsEksKubernetesRuntimeInstance,
				)
				if err != nil {
					log.Error(err, "failed to update aws eks kubernetes runtime instance to mark as reconciled")
					r.UnlockAndRequeue(&awsEksKubernetesRuntimeInstance, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"aws eks kubernetes runtime instance marked as reconciled in API",
					"aws eks kubernetes runtime instanceName", updatedAwsEksKubernetesRuntimeInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&awsEksKubernetesRuntimeInstance, lockReleased, msg, true); !ok {
				log.V(1).Info("aws eks kubernetes runtime instance remains locked - will unlock when TTL expires")
			} else {
				log.V(1).Info("aws eks kubernetes runtime instance unlocked")
			}

			log.Info("aws eks kubernetes runtime instance successfully reconciled")
			msg.Ack()
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
