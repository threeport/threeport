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

// MetricsInstanceReconciler reconciles system state when a MetricsInstance
// is created, updated or deleted.
func MetricsInstanceReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("metrics instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// determine the correct object version from the notification
			var metricsInstance tpapi_lib.ReconciledThreeportApiObject
			switch notif.ObjectVersion {
			case "v0":
				metricsInstance = &api_v0.MetricsInstance{}
			default:
				log.Error(errors.New("received unrecognized version of metrics instance object"), "")
				r.RequeueRaw(msg)
				log.V(1).Info("metrics instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			if err := metricsInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("metrics instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("metricsInstanceID", metricsInstance.GetId())

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(metricsInstance)
			if locked || ok == false {
				r.Requeue(metricsInstance, requeueDelay, msg)
				log.V(1).Info("metrics instance reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of metrics instance")
					r.UnlockAndRequeue(metricsInstance, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for metrics instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(metricsInstance); !ok {
				r.Requeue(metricsInstance, requeueDelay, msg)
				log.V(1).Info("metrics instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object
			var latestMetricsInstance tpapi_lib.ReconciledThreeportApiObject
			var getLatestErr error
			switch notif.ObjectVersion {
			case "v0":
				latestObject, err := client_v0.GetMetricsInstanceByID(
					r.APIClient,
					r.APIServer,
					metricsInstance.GetId(),
				)
				latestMetricsInstance = latestObject
				getLatestErr = err
			default:
				getLatestErr = errors.New("received unrecognized version of metrics instance object")
			}

			// check if error is 404 - if object no longer exists, no need to requeue
			if errors.Is(getLatestErr, tpclient_lib.ErrObjectNotFound) {
				log.Info("object no longer exists - halting reconciliation")
				r.ReleaseLock(metricsInstance, lockReleased, msg, true)
				continue
			}
			if getLatestErr != nil {
				log.Error(getLatestErr, "failed to get metrics instance by ID from API")
				r.UnlockAndRequeue(metricsInstance, requeueDelay, lockReleased, msg)
				continue
			}
			metricsInstance = latestMetricsInstance

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				if metricsInstance.ScheduledForDeletion() != nil {
					log.Info("metrics instance scheduled for deletion - skipping create")
					break
				}
				var operationErr error
				var customRequeueDelay int64
				switch metricsInstance.GetVersion() {
				case "v0":
					requeueDelay, err := v0MetricsInstanceCreated(
						r,
						metricsInstance.(*api_v0.MetricsInstance),
						&log,
					)
					customRequeueDelay = requeueDelay
					operationErr = err
				default:
					operationErr = errors.New("unrecognized version of metrics instance encountered for creation")
				}
				if operationErr != nil {
					errorMsg := "failed to reconcile created metrics instance object"
					log.Error(operationErr, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&api_v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr(event.ReasonFailedCreate),
							Type:   util.Ptr(event.TypeNormal),
						},
						metricsInstance.GetId(),
						metricsInstance.GetVersion(),
						metricsInstance.GetType(),
						operationErr,
						&log,
					)
					r.UnlockAndRequeue(
						metricsInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						metricsInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				var operationErr error
				var customRequeueDelay int64
				switch metricsInstance.GetVersion() {
				case "v0":
					requeueDelay, err := v0MetricsInstanceUpdated(
						r,
						metricsInstance.(*api_v0.MetricsInstance),
						&log,
					)
					customRequeueDelay = requeueDelay
					operationErr = err
				default:
					operationErr = errors.New("unrecognized version of metrics instance encountered for creation")
				}
				if operationErr != nil {
					errorMsg := "failed to reconcile updated metrics instance object"
					log.Error(operationErr, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&api_v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr(event.ReasonFailedUpdate),
							Type:   util.Ptr(event.TypeNormal),
						},
						metricsInstance.GetId(),
						metricsInstance.GetVersion(),
						metricsInstance.GetType(),
						operationErr,
						&log,
					)
					r.UnlockAndRequeue(
						metricsInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						metricsInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				var operationErr error
				var customRequeueDelay int64
				switch metricsInstance.GetVersion() {
				case "v0":
					requeueDelay, err := v0MetricsInstanceDeleted(
						r,
						metricsInstance.(*api_v0.MetricsInstance),
						&log,
					)
					customRequeueDelay = requeueDelay
					operationErr = err
				default:
					operationErr = errors.New("unrecognized version of metrics instance encountered for creation")
				}
				if operationErr != nil {
					errorMsg := "failed to reconcile deleted metrics instance object"
					log.Error(operationErr, errorMsg)
					r.EventsRecorder.HandleEventOverride(
						&api_v0.Event{
							Note:   util.Ptr(errorMsg),
							Reason: util.Ptr(event.ReasonFailedDelete),
							Type:   util.Ptr(event.TypeNormal),
						},
						metricsInstance.GetId(),
						metricsInstance.GetVersion(),
						metricsInstance.GetType(),
						operationErr,
						&log,
					)
					r.UnlockAndRequeue(
						metricsInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("delete requeued for future reconciliation")
					r.UnlockAndRequeue(
						metricsInstance,
						customRequeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				deletionTimestamp := util.Ptr(time.Now().UTC())
				deletedMetricsInstance := api_v0.MetricsInstance{
					Common: api_v0.Common{ID: util.Ptr(metricsInstance.GetId())},
					Reconciliation: api_v0.Reconciliation{
						DeletionAcknowledged: deletionTimestamp,
						DeletionConfirmed:    deletionTimestamp,
						Reconciled:           util.Ptr(true),
					},
				}
				_, err = client_v0.UpdateMetricsInstance(
					r.APIClient,
					r.APIServer,
					&deletedMetricsInstance,
				)
				if err != nil {
					log.Error(err, "failed to update metrics instance to mark as deleted")
					r.UnlockAndRequeue(metricsInstance, requeueDelay, lockReleased, msg)
					continue
				}
				_, err = client_v0.DeleteMetricsInstance(
					r.APIClient,
					r.APIServer,
					metricsInstance.GetId(),
				)
				if err != nil {
					log.Error(err, "failed to delete metrics instance")
					r.UnlockAndRequeue(metricsInstance, requeueDelay, lockReleased, msg)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					metricsInstance,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue
			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				reconciledMetricsInstance := api_v0.MetricsInstance{
					Common:         api_v0.Common{ID: util.Ptr(metricsInstance.GetId())},
					Reconciliation: api_v0.Reconciliation{Reconciled: util.Ptr(true)},
				}
				updatedMetricsInstance, err := client_v0.UpdateMetricsInstance(
					r.APIClient,
					r.APIServer,
					&reconciledMetricsInstance,
				)
				if err != nil {
					log.Error(err, "failed to update metrics instance to mark as reconciled")
					r.UnlockAndRequeue(metricsInstance, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"metrics instance marked as reconciled in API",
					"metrics instanceName", updatedMetricsInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(metricsInstance, lockReleased, msg, true); !ok {
				log.Error(errors.New("metrics instance remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("metrics instance unlocked")
			}

			// log and record event for successful reconciliation
			successMsg := fmt.Sprintf(
				"metrics instance successfully reconciled for %s operation",
				strings.ToLower(string(notif.Operation)),
			)
			if err := r.EventsRecorder.RecordEvent(
				&api_v0.Event{
					Note:   util.Ptr(successMsg),
					Reason: util.Ptr(event.GetSuccessReasonForOperation(notif.Operation)),
					Type:   util.Ptr(event.TypeNormal),
				},
				metricsInstance.GetId(),
				metricsInstance.GetVersion(),
				metricsInstance.GetType(),
			); err != nil {
				log.Error(err, "failed to record event for successful metrics instance reconciliation")
			}
			log.Info(successMsg)
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
