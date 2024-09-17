// generated by 'threeport-sdk gen' - do not edit

package observability

import (
	"errors"
	"fmt"
	tpapi_lib "github.com/threeport/threeport/pkg/api/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	tpclient_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client_v0 "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	event "github.com/threeport/threeport/pkg/event/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// ObservabilityDashboardInstanceReconciler reconciles system state when a ObservabilityDashboardInstance
// is created, updated or deleted.
func ObservabilityDashboardInstanceReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("observability dashboard instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// determine the correct object version from the notification
			var observabilityDashboardInstance tpapi_lib.ReconciledThreeportApiObject
			switch notif.ObjectVersion {
			case "v0":
				observabilityDashboardInstance = &api_v0.ObservabilityDashboardInstance{}
			default:
				log.Error(errors.New("received unrecognized version of observability dashboard instance object"), "")
				r.RequeueRaw(msg)
				log.V(1).Info("observability dashboard instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			if err := observabilityDashboardInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("observability dashboard instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("observabilityDashboardInstanceID", observabilityDashboardInstance.GetId())

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(observabilityDashboardInstance)
			if locked || ok == false {
				r.Requeue(observabilityDashboardInstance, requeueDelay, msg)
				log.V(1).Info("observability dashboard instance reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of observability dashboard instance")
					r.UnlockAndRequeue(observabilityDashboardInstance, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for observability dashboard instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(observabilityDashboardInstance); !ok {
				r.Requeue(observabilityDashboardInstance, requeueDelay, msg)
				log.V(1).Info("observability dashboard instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object
			var latestObservabilityDashboardInstance tpapi_lib.ReconciledThreeportApiObject
			var getLatestErr error
			switch notif.ObjectVersion {
			case "v0":
				latestObject, err := client_v0.GetObservabilityDashboardInstanceByID(
					r.APIClient,
					r.APIServer,
					observabilityDashboardInstance.GetId(),
				)
				latestObservabilityDashboardInstance = latestObject
				getLatestErr = err
			default:
				getLatestErr = errors.New("received unrecognized version of observability dashboard instance object")
			}

			// check if error is 404 - if object no longer exists, no need to requeue
			if errors.Is(getLatestErr, tpclient_lib.ErrObjectNotFound) {
				log.Info("object no longer exists - halting reconciliation")
				r.ReleaseLock(observabilityDashboardInstance, lockReleased, msg, true)
				continue
			}
			if getLatestErr != nil {
				log.Error(getLatestErr, "failed to get observability dashboard instance by ID from API")
				r.UnlockAndRequeue(observabilityDashboardInstance, requeueDelay, lockReleased, msg)
				continue
			}
			observabilityDashboardInstance = latestObservabilityDashboardInstance

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if observabilityDashboardInstance.ScheduledForDeletion() != nil {
					log.Info("observability dashboard instance scheduled for deletion - skipping create")
					break
				}
				var operationErr error
				var customRequeueDelay int64
				switch observabilityDashboardInstance.GetVersion() {
				case "v0":
					requeueDelay, err := v0ObservabilityDashboardInstanceCreated(
						r,
						observabilityDashboardInstance.(*api_v0.ObservabilityDashboardInstance),
						&log,
					)
					customRequeueDelay = requeueDelay
					operationErr = err
				default:
					operationErr = errors.New("unrecognized version of observability dashboard instance encountered for creation")
				}
				if operationErr != nil {
					errorMsg := "failed to reconcile created observability dashboard instance object"
					log.Error(operationErr, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&api_v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr(event.ReasonFailedCreate),
							Type:   util.Ptr(event.TypeNormal),
						},
						observabilityDashboardInstance.GetId(),
						observabilityDashboardInstance.GetVersion(),
						observabilityDashboardInstance.GetType(),
						operationErr,
						&log,
					)
					r.UnlockAndRequeue(
						observabilityDashboardInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						observabilityDashboardInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				var operationErr error
				var customRequeueDelay int64
				switch observabilityDashboardInstance.GetVersion() {
				case "v0":
					requeueDelay, err := v0ObservabilityDashboardInstanceUpdated(
						r,
						observabilityDashboardInstance.(*api_v0.ObservabilityDashboardInstance),
						&log,
					)
					customRequeueDelay = requeueDelay
					operationErr = err
				default:
					operationErr = errors.New("unrecognized version of observability dashboard instance encountered for creation")
				}
				if operationErr != nil {
					errorMsg := "failed to reconcile created observability dashboard instance object"
					log.Error(operationErr, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&api_v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr(event.ReasonFailedUpdate),
							Type:   util.Ptr(event.TypeNormal),
						},
						observabilityDashboardInstance.GetId(),
						observabilityDashboardInstance.GetVersion(),
						observabilityDashboardInstance.GetType(),
						operationErr,
						&log,
					)
					r.UnlockAndRequeue(
						observabilityDashboardInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						observabilityDashboardInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				var operationErr error
				var customRequeueDelay int64
				switch observabilityDashboardInstance.GetVersion() {
				case "v0":
					requeueDelay, err := v0ObservabilityDashboardInstanceDeleted(
						r,
						observabilityDashboardInstance.(*api_v0.ObservabilityDashboardInstance),
						&log,
					)
					customRequeueDelay = requeueDelay
					operationErr = err
				default:
					operationErr = errors.New("unrecognized version of observability dashboard instance encountered for creation")
				}
				if operationErr != nil {
					errorMsg := "failed to reconcile created observability dashboard instance object"
					log.Error(operationErr, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&api_v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr(event.ReasonFailedDelete),
							Type:   util.Ptr(event.TypeNormal),
						},
						observabilityDashboardInstance.GetId(),
						observabilityDashboardInstance.GetVersion(),
						observabilityDashboardInstance.GetType(),
						operationErr,
						&log,
					)
					r.UnlockAndRequeue(
						observabilityDashboardInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("delete requeued for future reconciliation")
					r.UnlockAndRequeue(
						observabilityDashboardInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				deletionTimestamp := util.Ptr(time.Now().UTC())
				deletedObservabilityDashboardInstance := api_v0.ObservabilityDashboardInstance{
					Common: api_v0.Common{ID: util.Ptr(observabilityDashboardInstance.GetId())},
					Reconciliation: api_v0.Reconciliation{
						DeletionAcknowledged: deletionTimestamp,
						DeletionConfirmed:    deletionTimestamp,
						Reconciled:           util.Ptr(true),
					},
				}
				_, err = client_v0.UpdateObservabilityDashboardInstance(
					r.APIClient,
					r.APIServer,
					&deletedObservabilityDashboardInstance,
				)
				if err != nil {
					log.Error(err, "failed to update observability dashboard instance to mark as deleted")
					r.UnlockAndRequeue(observabilityDashboardInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client_v0.DeleteObservabilityDashboardInstance(
					r.APIClient,
					r.APIServer,
					observabilityDashboardInstance.GetId(),
				)
				if err != nil {
					log.Error(err, "failed to delete observability dashboard instance")
					r.UnlockAndRequeue(observabilityDashboardInstance, requeueDelay, lockReleased, msg)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					observabilityDashboardInstance,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue
			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				reconciledObservabilityDashboardInstance := api_v0.ObservabilityDashboardInstance{
					Common:         api_v0.Common{ID: util.Ptr(observabilityDashboardInstance.GetId())},
					Reconciliation: api_v0.Reconciliation{Reconciled: util.Ptr(true)},
				}
				updatedObservabilityDashboardInstance, err := client_v0.UpdateObservabilityDashboardInstance(
					r.APIClient,
					r.APIServer,
					&reconciledObservabilityDashboardInstance,
				)
				if err != nil {
					log.Error(err, "failed to update observability dashboard instance to mark as reconciled")
					r.UnlockAndRequeue(observabilityDashboardInstance, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"observability dashboard instance marked as reconciled in API",
					"observability dashboard instanceName", updatedObservabilityDashboardInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(observabilityDashboardInstance, lockReleased, msg, true); !ok {
				log.Error(errors.New("observability dashboard instance remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("observability dashboard instance unlocked")
			}

			// log and record event for successful reconciliation
			successMsg := fmt.Sprintf(
				"observability dashboard instance successfully reconciled for %s operation",
				strings.ToLower(string(notif.Operation)),
			)
			if err := r.EventsRecorder.RecordEvent(
				&api_v0.Event{
					Note:   util.Ptr(successMsg),
					Reason: util.Ptr(event.GetSuccessReasonForOperation(notif.Operation)),
					Type:   util.Ptr(event.TypeNormal),
				},
				observabilityDashboardInstance.GetId(),
				observabilityDashboardInstance.GetVersion(),
				observabilityDashboardInstance.GetType(),
			); err != nil {
				log.Error(err, "failed to record event for successful observability dashboard instance reconciliation")
			}
			log.Info(successMsg)
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
