// generated by 'threeport-sdk gen' for controller scaffolding - do not edit

package workload

import (
	"errors"
	"fmt"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	v1 "github.com/threeport/threeport/pkg/api/v1"
	client "github.com/threeport/threeport/pkg/client/v0"
	client_v1 "github.com/threeport/threeport/pkg/client/v1"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"os"
	"os/signal"
	"syscall"
	"time"
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
					"msgData", string(msg.Data),
				)
				r.RequeueRaw(msg)
				log.V(1).Info("workload instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var workloadInstance v1.WorkloadInstance
			if err := workloadInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("workload instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("workloadInstanceID", workloadInstance.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(&workloadInstance)
			if locked || ok == false {
				r.Requeue(&workloadInstance, requeueDelay, msg)
				log.V(1).Info("workload instance reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of workload instance")
					r.UnlockAndRequeue(&workloadInstance, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for workload instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&workloadInstance); !ok {
				r.Requeue(&workloadInstance, requeueDelay, msg)
				log.V(1).Info("workload instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object
			latestWorkloadInstance, err := client_v1.GetWorkloadInstanceByID(
				r.APIClient,
				r.APIServer,
				*workloadInstance.ID,
			)
			// check if error is 404 - if object no longer exists, no need to requeue
			if errors.Is(err, client.ErrObjectNotFound) {
				log.Info(fmt.Sprintf(
					"object with ID %d no longer exists - halting reconciliation",
					*workloadInstance.ID,
				))
				r.ReleaseLock(&workloadInstance, lockReleased, msg, true)
				continue
			}
			if err != nil {
				log.Error(err, "failed to get workload instance by ID from API")
				r.UnlockAndRequeue(&workloadInstance, requeueDelay, lockReleased, msg)
				continue
			}
			workloadInstance = *latestWorkloadInstance

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if workloadInstance.DeletionScheduled != nil {
					log.Info("workload instance scheduled for deletion - skipping create")
					break
				}
				customRequeueDelay, err := workloadInstanceCreated(r, &workloadInstance, &log)
				if err != nil {
					errorMsg := "failed to reconcile created workload instance object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("WorkloadInstanceNotCreated"),
							Type:   util.Ptr("Normal"),
						},
						workloadInstance.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&workloadInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						&workloadInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				customRequeueDelay, err := workloadInstanceUpdated(r, &workloadInstance, &log)
				if err != nil {
					errorMsg := "failed to reconcile updated workload instance object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("WorkloadInstanceNotUpdated"),
							Type:   util.Ptr("Normal"),
						},
						workloadInstance.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&workloadInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						&workloadInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				customRequeueDelay, err := workloadInstanceDeleted(r, &workloadInstance, &log)
				if err != nil {
					errorMsg := "failed to reconcile deleted workload instance object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("WorkloadInstanceNotUpdated"),
							Type:   util.Ptr("Normal"),
						},
						workloadInstance.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&workloadInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("deletion requeued for future reconciliation")
					r.UnlockAndRequeue(
						&workloadInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				deletionTimestamp := util.TimePtr(time.Now().UTC())
				deletedWorkloadInstance := v1.WorkloadInstance{
					Common: v0.Common{ID: workloadInstance.ID},
					Reconciliation: v0.Reconciliation{
						DeletionAcknowledged: deletionTimestamp,
						DeletionConfirmed:    deletionTimestamp,
						Reconciled:           util.BoolPtr(true),
					},
				}
				if err != nil {
					log.Error(err, "failed to update workload instance to mark as reconciled")
					r.UnlockAndRequeue(&workloadInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client_v1.UpdateWorkloadInstance(
					r.APIClient,
					r.APIServer,
					&deletedWorkloadInstance,
				)
				if err != nil {
					log.Error(err, "failed to update workload instance to mark as deleted")
					r.UnlockAndRequeue(&workloadInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.DeleteWorkloadInstance(
					r.APIClient,
					r.APIServer,
					*workloadInstance.ID,
				)
				if err != nil {
					log.Error(err, "failed to delete workload instance")
					r.UnlockAndRequeue(&workloadInstance, requeueDelay, lockReleased, msg)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&workloadInstance,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				reconciledWorkloadInstance := v1.WorkloadInstance{
					Common:         v0.Common{ID: workloadInstance.ID},
					Reconciliation: v0.Reconciliation{Reconciled: util.BoolPtr(true)},
				}
				updatedWorkloadInstance, err := client_v1.UpdateWorkloadInstance(
					r.APIClient,
					r.APIServer,
					&reconciledWorkloadInstance,
				)
				if err != nil {
					log.Error(err, "failed to update workload instance to mark as reconciled")
					r.UnlockAndRequeue(&workloadInstance, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"workload instance marked as reconciled in API",
					"workload instanceName", updatedWorkloadInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&workloadInstance, lockReleased, msg, true); !ok {
				log.Error(errors.New("workload instance remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("workload instance unlocked")
			}

			successMsg := "workload instance successfully reconciled for %s operation"
			log.Info(fmt.Sprintf(
				successMsg,
				notif.Operation,
			))
			if err := r.EventsRecorder.RecordEvent(
				&v0.Event{
					Note:   util.Ptr(successMsg),
					Reason: util.Ptr("WorkloadInstanceSuccessfullyReconciled"),
					Type:   util.Ptr("Normal"),
				},
				workloadInstance.ID,
			); err != nil {
				log.Error(err, "failed to record event for successful workload instance reconciliation")
			}
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
