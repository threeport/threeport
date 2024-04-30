// generated by 'threeport-sdk gen' for controller scaffolding - do not edit

package helmworkload

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

// HelmWorkloadInstanceReconciler reconciles system state when a HelmWorkloadInstance
// is created, updated or deleted.
func HelmWorkloadInstanceReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("helm workload instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var helmWorkloadInstance v0.HelmWorkloadInstance
			if err := helmWorkloadInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("helm workload instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("helmWorkloadInstanceID", helmWorkloadInstance.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(&helmWorkloadInstance)
			if locked || ok == false {
				r.Requeue(&helmWorkloadInstance, requeueDelay, msg)
				log.V(1).Info("helm workload instance reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of helm workload instance")
					r.UnlockAndRequeue(&helmWorkloadInstance, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for helm workload instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&helmWorkloadInstance); !ok {
				r.Requeue(&helmWorkloadInstance, requeueDelay, msg)
				log.V(1).Info("helm workload instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object
			latestHelmWorkloadInstance, err := client.GetHelmWorkloadInstanceByID(
				r.APIClient,
				r.APIServer,
				*helmWorkloadInstance.ID,
			)
			// check if error is 404 - if object no longer exists, no need to requeue
			if errors.Is(err, client.ErrObjectNotFound) {
				log.Info(fmt.Sprintf(
					"object with ID %d no longer exists - halting reconciliation",
					*helmWorkloadInstance.ID,
				))
				r.ReleaseLock(&helmWorkloadInstance, lockReleased, msg, true)
				continue
			}
			if err != nil {
				log.Error(err, "failed to get helm workload instance by ID from API")
				r.UnlockAndRequeue(&helmWorkloadInstance, requeueDelay, lockReleased, msg)
				continue
			}
			helmWorkloadInstance = *latestHelmWorkloadInstance

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if helmWorkloadInstance.DeletionScheduled != nil {
					log.Info("helm workload instance scheduled for deletion - skipping create")
					break
				}
				customRequeueDelay, err := helmWorkloadInstanceCreated(r, &helmWorkloadInstance, &log)
				if err != nil {
					errorMsg := "failed to reconcile created helm workload instance object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("HelmWorkloadInstanceNotCreated"),
							Type:   util.Ptr("Normal"),
						},
						helmWorkloadInstance.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&helmWorkloadInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						&helmWorkloadInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				customRequeueDelay, err := helmWorkloadInstanceUpdated(r, &helmWorkloadInstance, &log)
				if err != nil {
					errorMsg := "failed to reconcile updated helm workload instance object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("HelmWorkloadInstanceNotUpdated"),
							Type:   util.Ptr("Normal"),
						},
						helmWorkloadInstance.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&helmWorkloadInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						&helmWorkloadInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				customRequeueDelay, err := helmWorkloadInstanceDeleted(r, &helmWorkloadInstance, &log)
				if err != nil {
					errorMsg := "failed to reconcile deleted helm workload instance object"
					log.Error(err, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr("HelmWorkloadInstanceNotUpdated"),
							Type:   util.Ptr("Normal"),
						},
						helmWorkloadInstance.ID,
						err,
						&log,
					)
					r.UnlockAndRequeue(
						&helmWorkloadInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("deletion requeued for future reconciliation")
					r.UnlockAndRequeue(
						&helmWorkloadInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				deletionTimestamp := util.TimePtr(time.Now().UTC())
				deletedHelmWorkloadInstance := v0.HelmWorkloadInstance{
					Common: v0.Common{ID: helmWorkloadInstance.ID},
					Reconciliation: v0.Reconciliation{
						DeletionAcknowledged: deletionTimestamp,
						DeletionConfirmed:    deletionTimestamp,
						Reconciled:           util.BoolPtr(true),
					},
				}
				if err != nil {
					log.Error(err, "failed to update helm workload instance to mark as reconciled")
					r.UnlockAndRequeue(&helmWorkloadInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.UpdateHelmWorkloadInstance(
					r.APIClient,
					r.APIServer,
					&deletedHelmWorkloadInstance,
				)
				if err != nil {
					log.Error(err, "failed to update helm workload instance to mark as deleted")
					r.UnlockAndRequeue(&helmWorkloadInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client.DeleteHelmWorkloadInstance(
					r.APIClient,
					r.APIServer,
					*helmWorkloadInstance.ID,
				)
				if err != nil {
					log.Error(err, "failed to delete helm workload instance")
					r.UnlockAndRequeue(&helmWorkloadInstance, requeueDelay, lockReleased, msg)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&helmWorkloadInstance,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				reconciledHelmWorkloadInstance := v0.HelmWorkloadInstance{
					Common:         v0.Common{ID: helmWorkloadInstance.ID},
					Reconciliation: v0.Reconciliation{Reconciled: util.BoolPtr(true)},
				}
				updatedHelmWorkloadInstance, err := client.UpdateHelmWorkloadInstance(
					r.APIClient,
					r.APIServer,
					&reconciledHelmWorkloadInstance,
				)
				if err != nil {
					log.Error(err, "failed to update helm workload instance to mark as reconciled")
					r.UnlockAndRequeue(&helmWorkloadInstance, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"helm workload instance marked as reconciled in API",
					"helm workload instanceName", updatedHelmWorkloadInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&helmWorkloadInstance, lockReleased, msg, true); !ok {
				log.Error(errors.New("helm workload instance remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("helm workload instance unlocked")
			}

			successMsg := fmt.Sprintf(
				"helm workload instance successfully reconciled for %s operation",
				strings.ToLower(string(notif.Operation)),
			)
			if err := r.EventsRecorder.RecordEvent(
				&v0.Event{
					Note:   util.Ptr(successMsg),
					Reason: util.Ptr("HelmWorkloadInstanceSuccessfullyReconciled"),
					Type:   util.Ptr("Normal"),
				},
				helmWorkloadInstance.ID,
			); err != nil {
				log.Error(err, "failed to record event for successful helm workload instance reconciliation")
			}
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
