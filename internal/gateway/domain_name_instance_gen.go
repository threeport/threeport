// generated by 'threeport-codegen controller' - do not edit

package gateway

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

// DomainNameInstanceReconciler reconciles system state when a DomainNameInstance
// is created, updated or deleted.
func DomainNameInstanceReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("domain name instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was sent in the notification
			var domainNameInstance v0.DomainNameInstance
			if err := domainNameInstance.DecodeNotifObject(notif.Object); err != nil {
				log.Error(err, "failed to marshal object map from consumed notification message")
				r.RequeueRaw(msg)
				log.V(1).Info("domain name instance reconciliation requeued with identical payload and fixed delay")
				continue
			}
			log = log.WithValues("domainNameInstanceID", domainNameInstance.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.CreationTime,
			)

			// check for lock on object
			locked, ok := r.CheckLock(&domainNameInstance)
			if locked || ok == false {
				r.Requeue(&domainNameInstance, requeueDelay, msg)
				log.V(1).Info("domain name instance reconciliation requeued")
				continue
			}

			// set up handler to unlock and requeue on termination signal
			go func() {
				select {
				case <-osSignals:
					log.V(1).Info("received termination signal, performing unlock and requeue of domain name instance")
					r.UnlockAndRequeue(&domainNameInstance, requeueDelay, lockReleased, msg)
				case <-lockReleased:
					log.V(1).Info("reached end of reconcile loop for domain name instance, closing out signal handler")
				}
			}()

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&domainNameInstance); !ok {
				r.Requeue(&domainNameInstance, requeueDelay, msg)
				log.V(1).Info("domain name instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object
			latestDomainNameInstance, err := client.GetDomainNameInstanceByID(
				r.APIClient,
				r.APIServer,
				*domainNameInstance.ID,
			)
			// check if error is 404 - if object no longer exists, no need to requeue
			if errors.Is(err, client.ErrorObjectNotFound) {
				log.Info(fmt.Sprintf(
					"object with ID %d no longer exists - halting reconciliation",
					*domainNameInstance.ID,
				))
				r.ReleaseLock(&domainNameInstance, lockReleased, msg, true)
				continue
			}
			if err != nil {
				log.Error(err, "failed to get domain name instance by ID from API")
				r.UnlockAndRequeue(&domainNameInstance, requeueDelay, lockReleased, msg)
				continue
			}
			domainNameInstance = *latestDomainNameInstance

			// determine which operation and act accordingly
			switch notif.Operation {
			case notifications.NotificationOperationCreated:
				customRequeueDelay, err := domainNameInstanceCreated(r, &domainNameInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile created domain name instance object")
					r.UnlockAndRequeue(
						&domainNameInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("create requeued for future reconciliation")
					r.UnlockAndRequeue(
						&domainNameInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationUpdated:
				customRequeueDelay, err := domainNameInstanceUpdated(r, &domainNameInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile updated domain name instance object")
					r.UnlockAndRequeue(
						&domainNameInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("update requeued for future reconciliation")
					r.UnlockAndRequeue(
						&domainNameInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			case notifications.NotificationOperationDeleted:
				customRequeueDelay, err := domainNameInstanceDeleted(r, &domainNameInstance, &log)
				if err != nil {
					log.Error(err, "failed to reconcile deleted domain name instance object")
					r.UnlockAndRequeue(
						&domainNameInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
				if customRequeueDelay != 0 {
					log.Info("deletion requeued for future reconciliation")
					r.UnlockAndRequeue(
						&domainNameInstance,
						requeueDelay,
						lockReleased,
						msg,
					)
					continue
				}
			default:
				log.Error(
					errors.New("unrecognized notifcation operation"),
					"notification included an invalid operation",
				)
				r.UnlockAndRequeue(
					&domainNameInstance,
					requeueDelay,
					lockReleased,
					msg,
				)
				continue

			}

			// set the object's Reconciled field to true if not deleted
			if notif.Operation != notifications.NotificationOperationDeleted {
				objectReconciled := true
				reconciledDomainNameInstance := v0.DomainNameInstance{
					Common:         v0.Common{ID: domainNameInstance.ID},
					Reconciliation: v0.Reconciliation{Reconciled: &objectReconciled},
				}
				updatedDomainNameInstance, err := client.UpdateDomainNameInstance(
					r.APIClient,
					r.APIServer,
					&reconciledDomainNameInstance,
				)
				if err != nil {
					log.Error(err, "failed to update domain name instance to mark as reconciled")
					r.UnlockAndRequeue(&domainNameInstance, requeueDelay, lockReleased, msg)
					continue
				}
				log.V(1).Info(
					"domain name instance marked as reconciled in API",
					"domain name instanceName", updatedDomainNameInstance.Name,
				)
			}

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&domainNameInstance, lockReleased, msg, true); !ok {
				log.Error(errors.New("domain name instance remains locked - will unlock when TTL expires"), "")
			} else {
				log.V(1).Info("domain name instance unlocked")
			}

			log.Info(fmt.Sprintf(
				"domain name instance successfully reconciled for %s operation",
				notif.Operation,
			))
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
