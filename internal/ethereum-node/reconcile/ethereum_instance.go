package reconcile

import (
	"fmt"

	"github.com/mitchellh/mapstructure"

	//kubecluster "github.com/threeport/threeport/internal/cluster/kube"
	//kubeworkload "github.com/threeport/threeport/internal/workload/kube"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/controller"
	"github.com/threeport/threeport/pkg/notifications"
)

// EthereumNodeInstanceReconciler reconciles system state when a EthereumNodeInstance
// is created, updated or deleted.  It references the EthereumNodeDefinitions
// and manages them in the configured workload cluster.
func EthereumNodeInstanceReconciler(r *controller.Reconciler) {
	r.ShutdownWait.Add(1)
	reconcilerLog := r.Log.WithValues("reconcilerName", r.Name)
	reconcilerLog.Info("reconciler started")
	shutdown := false

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
				log.V(1).Info("ethereum node definition reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was created
			var ethereumNodeInstance v0.EthereumNodeInstance
			mapstructure.Decode(notif.Object, &ethereumNodeInstance)
			log = log.WithValues(
				"ethereumNodeInstanceID", ethereumNodeInstance.ID,
				"clusterInstanceID", ethereumNodeInstance.ClusterInstanceID,
				"ethereumNodeDefinitionID", ethereumNodeInstance.EthereumNodeDefinitionID,
			)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.LastRequeueDelay,
				controller.DefaultInitialRequeueDelay,
				controller.DefaultMaxRequeueDelay,
			)

			// build the notif payload for requeues
			notifPayload, err := ethereumNodeInstance.NotificationPayload(true, requeueDelay)
			if err != nil {
				log.Error(err, "failed to build notification payload for requeue")
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("ethereum node instance reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// check for lock on object
			locked, ok := r.CheckLock(&ethereumNodeInstance)
			if locked || !ok {
				go r.Requeue(&ethereumNodeInstance, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("ethereum node instance reconciliation requeued")
				continue
			}

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&ethereumNodeInstance); !ok {
				go r.Requeue(&ethereumNodeInstance, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("ethereum node instance reconciliation requeued")
				continue
			}

			// retrieve latest version of object if requeued
			if notif.Requeue {
				latestEthereumNodeInstance, err := client.GetEthereumNodeInstanceByID(
					*ethereumNodeInstance.ID,
					r.APIServer,
					"",
				)
				if err != nil {
					log.Error(err, "failed to get ethereum node instance by ID from API")
					r.UnlockAndRequeue(&ethereumNodeInstance, msg.Subject, notifPayload, requeueDelay)
					continue
				}
				ethereumNodeInstance = *latestEthereumNodeInstance
			}

			// ensure ethereum node definition is reconciled before working on
			// instance for it
			ethereumNodeDefinition, err := client.GetEthereumNodeDefinitionByID(
				*ethereumNodeInstance.EthereumNodeDefinitionID,
				r.APIServer,
				"",
			)
			if err != nil {
				log.Error(
					err, "failed to get ethereum node definition by ethereum node definition ID",
					"ethereumNodeDefinitionID", *ethereumNodeInstance.EthereumNodeDefinitionID,
				)
				r.UnlockAndRequeue(&ethereumNodeInstance, msg.Subject, notifPayload, requeueDelay)
				continue
			}
			if ethereumNodeDefinition.Reconciled != nil && !*ethereumNodeDefinition.Reconciled {
				log.V(1).Info("ethereum node definition not yet reconciled - requeueing ethereum node instance")
				r.UnlockAndRequeue(&ethereumNodeInstance, msg.Subject, notifPayload, requeueDelay)
				continue
			}

			// use ethereum node definition ID to get workload resource definitions
			workloadResourceDefinitions, err := client.GetEthereumNodeDefinitionByID(
				*ethereumNodeInstance.EthereumNodeDefinitionID,
				r.APIServer,
				"",
			)
			log.V(1).Info(
				"ethereum node definitions retrieved",
				"workloadResourceDefinitions", fmt.Sprintf("%+v\n", workloadResourceDefinitions),
				"ethereumNodeInstanceID", ethereumNodeInstance.ID,
			)
			if err != nil {
				log.Error(
					err, "failed to get workload resource definitions by ethereum node definition ID",
					"ethereumNodeDefinitionID", *ethereumNodeInstance.EthereumNodeDefinitionID,
				)
				r.UnlockAndRequeue(&ethereumNodeInstance, msg.Subject, notifPayload, requeueDelay)
				continue
			}

			// create workload instance
			workloadInstance := v0.WorkloadInstance {
				Instance: v0.Instance{
					Name: ethereumNodeInstance.Name,
					UserID: ethereumNodeInstance.UserID,
					CompanyID: ethereumNodeInstance.CompanyID,
				},
				ClusterInstanceID: ethereumNodeInstance.ClusterInstanceID,
				WorkloadDefinitionID: ethereumNodeDefinition.WorkloadDefinitionID,
			}

			_, err = client.CreateWorkloadInstance(
				&workloadInstance,
				r.APIServer,
				"",
			)
			if err != nil {
				log.Error(err, "failed to create workload instance")
				r.UnlockAndRequeue(&ethereumNodeInstance, msg.Subject, notifPayload, requeueDelay)
				continue
			}
			log.V(1).Info(
				"workload instance created in API",
				"workloadInstanceName", workloadInstance.Name,
			)


			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&ethereumNodeInstance); !ok {
				log.V(1).Info("ethereum node instance remains locked - will unlock when TTL expires")
			} else {
				log.V(1).Info("ethereum node instance unlocked")
			}

			log.Info("ethereum node instance successfully reconciled")
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
