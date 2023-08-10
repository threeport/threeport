// generated by 'threeport-codegen controller' - do not edit

package workload

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

// WorkloadInstanceReconciler reconciles system state when a WorkloadInstance
// is created, updated or deleted.
func WorkloadInstanceReconciler(r *controller.Reconciler) {
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
					"msgSubject", msg.Subject,
					"msgData", string(msg.Data),
				)
				go r.RequeueRaw(msg)
				log.V(1).Info("workload instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var workloadInstance v0.WorkloadInstance
			if err := workloadInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				go r.RequeueRaw(msg)
				log.V(1).Info("workload instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("workloadInstanceID", workloadInstance.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
				controller.DefaultInitialRequeueDelay,
				controller.DefaultMaxRequeueDelay,
			)

			// build the notif payload for requeues
			notifPayload, err := workloadInstance.NotificationPayload(
				notif.Operation,
				true,
				requeueDelay,
			)
			if err != nil {
				log.Error(err, "failed to build notification payload for requeue")
				go r.RequeueRaw(msg)
				log.V(1).Info("workload instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// check for lock on object
			locked, ok := r.CheckLock(&workloadInstance)
			if locked || ok == false {
				go r.Requeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("workload instance reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of workload instance")
					r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for workload instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&workloadInstance); !ok {
				go r.Requeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("workload instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object if requeued unless object was
			// deleted (in which case we have the latest version)
			if notif.Requeue && notif.Operation != notifications.NotificationOperationDeleted {
				latestWorkloadInstance, err := client.GetWorkloadInstanceByID(
					r.APIClient,
					r.APIServer,
					*workloadInstance.ID,
				)
				// check if error is 404 - if object no longer exists, no need to requeue
				if errors.Is(err, client.ErrorObjectNotFound) {
					log.Info(fmt.Sprintf(
						"object with ID %d no longer exists - halting reconciliation",
						*workloadInstance.ID,
					))
					r.ReleaseLock(&workloadInstance, lockReleased)
					continue
				}
				if err != nil {
					log.Error(err, "failed to get workload instance by ID from API")
					r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay, lockReleased, msg)
					continue
				}
				workloadInstance = *latestWorkloadInstance
			}

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if err := workloadInstanceCreated(r, &workloadInstance, &log); err != nil {
					log.Error(err, "failed to reconcile created workload instance object")
					r.UnlockAndRequeue(
						&workloadInstance,
						msg.Subject,
						notifPayload,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				if err := workloadInstanceUpdated(r, &workloadInstance, &log); err != nil {
					log.Error(err, "failed to reconcile updated workload instance object")
					r.UnlockAndRequeue(
						&workloadInstance,
						msg.Subject,
						notifPayload,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				if err := workloadInstanceDeleted(r, &workloadInstance, &log); err != nil {
					log.Error(err, "failed to reconcile deleted workload instance object")
					r.UnlockAndRequeue(
						&workloadInstance,
						msg.Subject,
						notifPayload,
						requeueDelay,
						lockReleased,
						msg,
					)
				} else {
					r.ReleaseLock(&workloadInstance, lockReleased)
					log.Info("workload instance successfully reconciled")
				}
				continue
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&workloadInstance,
					msg.Subject,
					notifPayload,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				objectReconciled := true
				reconciledWorkloadInstance := v0.WorkloadInstance{
					Common:     v0.Common{ID: workloadInstance.ID},
					Reconciled: &objectReconciled,
				}
				updatedWorkloadInstance, err := client.UpdateWorkloadInstance(
					r.APIClient,
					r.APIServer,
					&reconciledWorkloadInstance,
				)
				if err != nil {
					log.Error(err, "failed to update workload instance to mark as reconciled")
					r.UnlockAndRequeue(&workloadInstance, msg.Subject, notifPayload, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"workload instance marked as reconciled in API",
					"workload instanceName", updatedWorkloadInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&workloadInstance, lockReleased); !ok {
				log.V(1).Info("workload instance remains locked - will unlock when TTL expires")
			} else {
				log.V(1).Info("workload instance unlocked")
			}

			log.Info("workload instance successfully reconciled")
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
