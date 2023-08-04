// generated by 'threeport-codegen controller' - do not edit

package workload

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

// WorkloadDefinitionReconciler reconciles system state when a WorkloadDefinition
// is created, updated or deleted.
func WorkloadDefinitionReconciler(r *controller.Reconciler) {
	r.ShutdownWait.Add(1)
	reconcilerLog := r.Log.WithValues("reconcilerName", r.Name)
	reconcilerLog.Info("reconciler started")
	shutdown := false

	var requeueDelay int64
	var notifPayload *[]byte
	// var workloadDefinition v0.WorkloadDefinition
	// var msg *nats.Msg
	var log logr.Logger

	// Create a channel to receive OS signals
	sigs := make(chan os.Signal, 1)
	endReconcile := make(chan bool, 1)

	// Register the channel to receive SIGINT signals
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	for {
		// create a fresh log object per reconciliation loop so we don't
		// accumulate values across multiple loops
		log = r.Log.WithValues("reconcilerName", r.Name)

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
				log.V(1).Info("workload definition reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var workloadDefinition v0.WorkloadDefinition
			if err := workloadDefinition.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("workload definition reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("workloadDefinitionID", workloadDefinition.ID)

			// back off the requeue delay as needed
			requeueDelay = controller.SetRequeueDelay(
				notif.LastRequeueDelay,
				controller.DefaultInitialRequeueDelay,
				controller.DefaultMaxRequeueDelay,
			)

			// build the notif payload for requeues
			notifPayload, err = workloadDefinition.NotificationPayload(
				notif.Operation,
				true,
				requeueDelay,
			)
			if err != nil {
				log.Error(err, "failed to build notification payload for requeue")
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("workload definition reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// check for lock on object
			locked, ok := r.CheckLock(&workloadDefinition)
			if locked || ok == false {
				go r.Requeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("workload definition reconciliation requeued")
				continue
			}

			// Run a goroutine to handle the signal. It will block until it receives a signal

			go func() {
				select {
				case <-sigs:
					log.Info("Received Ctrl+C, attempting to release lock and requeue...")

					if notifPayload != nil && msg != nil {
						r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
						log.Info("workload definition lock released and requeued")
					}
					os.Exit(0)
				case <-endReconcile:
					log.Info("Reached end of reconcile, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&workloadDefinition); !ok {
				go r.Requeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("workload definition reconciliation requeued")
				continue
			}

			// retrieve latest version of object if requeued unless object was
			// deleted (in which case we have the latest version)
			if notif.Requeue && notif.Operation != notifications.NotificationOperationDeleted {
				latestWorkloadDefinition, err := client.GetWorkloadDefinitionByID(
					r.APIClient,
					r.APIServer,
					*workloadDefinition.ID,
				)
				// check if error is 404 - if object no longer exists, no need to requeue
				if errors.Is(err, client.ErrorObjectNotFound) {
					log.Info(fmt.Sprintf(
						"object with ID %d no longer exists - halting reconciliation",
						*workloadDefinition.ID,
					))
					r.ReleaseLock(&workloadDefinition)
					continue
				}
				if err != nil {
					log.Error(err, "failed to get workload definition by ID from API")
					r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
					continue
				}
				workloadDefinition = *latestWorkloadDefinition
			}

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if err := workloadDefinitionCreated(r, &workloadDefinition, &log); err != nil {
					log.Error(err, "failed to reconcile created workload definition object")
					r.UnlockAndRequeue(
						&workloadDefinition,
						msg.Subject,
						notifPayload,
						requeueDelay,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				if err := workloadDefinitionUpdated(r, &workloadDefinition, &log); err != nil {
					log.Error(err, "failed to reconcile updated workload definition object")
					r.UnlockAndRequeue(
						&workloadDefinition,
						msg.Subject,
						notifPayload,
						requeueDelay,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				if err := workloadDefinitionDeleted(r, &workloadDefinition, &log); err != nil {
					log.Error(err, "failed to reconcile deleted workload definition object")
					r.UnlockAndRequeue(
						&workloadDefinition,
						msg.Subject,
						notifPayload,
						requeueDelay,
					)
				} else {
					r.ReleaseLock(&workloadDefinition)
					endReconcile <- true
					log.Info("workload definition successfully reconciled")
				}
				continue
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&workloadDefinition,
					msg.Subject,
					notifPayload,
					requeueDelay,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				objectReconciled := true
				reconciledWorkloadDefinition := v0.WorkloadDefinition{
					Common:     v0.Common{ID: workloadDefinition.ID},
					Reconciled: &objectReconciled,
				}
				updatedWorkloadDefinition, err := client.UpdateWorkloadDefinition(
					r.APIClient,
					r.APIServer,
					&reconciledWorkloadDefinition,
				)
				if err != nil {
					log.Error(err, "failed to update workload definition to mark as reconciled")
					r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
					continue
				}
				log.V(1).Info(
					"workload definition marked as reconciled in API",
					"workload definitionName", updatedWorkloadDefinition.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&workloadDefinition); !ok {
				log.V(1).Info("workload definition remains locked - will unlock when TTL expires")
			} else {
				log.V(1).Info("workload definition unlocked")
			}
			endReconcile <- true

			log.Info("workload definition successfully reconciled")
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
