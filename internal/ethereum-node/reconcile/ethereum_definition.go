package reconcile

import (
	"encoding/json"

	"github.com/mitchellh/mapstructure"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/controller"
	"github.com/threeport/threeport/pkg/notifications"
)

// EthereumNodeDefinitionReconciler reconciles system state when a EthereumNodeDefinition
// is created, updated or deleted.
func EthereumNodeDefinitionReconciler(r *controller.Reconciler) {
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

			// unmarshal notification from message data
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

			// decode the object that was created
			var ethereumNodeDefinition v0.EthereumNodeDefinition
			mapstructure.Decode(notif.Object, &ethereumNodeDefinition)
			log = log.WithValues("ethereumNodeDefinitionID", ethereumNodeDefinition.ID)

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.LastRequeueDelay,
				controller.DefaultInitialRequeueDelay,
				controller.DefaultMaxRequeueDelay,
			)

			// build the notif payload for requeues
			notifPayload, err := ethereumNodeDefinition.NotificationPayload(true, requeueDelay)
			if err != nil {
				log.Error(err, "failed to build notification payload for requeue")
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("ethereum node definition reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// check for lock on object
			locked, ok := r.CheckLock(&ethereumNodeDefinition)
			if locked || !ok {
				go r.Requeue(&ethereumNodeDefinition, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("ethereum node definition reconciliation requeued")
				continue
			}

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&ethereumNodeDefinition); !ok {
				go r.Requeue(&ethereumNodeDefinition, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("ethereum node definition reconciliation requeued")
				continue
			}

			// check for deletion
			if notif.Operation == "Deleted" {
				log.V(1).Info("received deleted notification")

				// _, err = client.DeleteWorkloadInstance(
				// 	&workloadInstance,
				// 	r.APIServer,
				// 	"",
				// )
				// if err != nil {
				// 	log.Error(err, "failed to update workload instance")
				// 	r.UnlockAndRequeue(&ethereumNodeInstance, msg.Subject, notifPayload, requeueDelay)
				// 	continue
				// }
				// log.V(1).Info(
				// 	"workload instance deleted in API",
				// 	"workloadInstanceName", workloadInstance.Name,
				// )
			}

			// if definition isn't being deleted, then we'll need to generate its manifests

			// retrieve latest version of object if requeued
			if notif.Requeue {
				latestEthereumNodeDefinition, err := client.GetEthereumNodeDefinitionByID(
					*ethereumNodeDefinition.ID,
					r.APIServer,
					"",
				)
				if err != nil {
					log.Error(err, "failed to get ethereum node definition by ID from API")
					r.UnlockAndRequeue(&ethereumNodeDefinition, msg.Subject, notifPayload, requeueDelay)
					continue
				}
				ethereumNodeDefinition = *latestEthereumNodeDefinition
			}

			// get manifest objects and marshal into json
			json, err := json.Marshal(GetManifestObjects(ethereumNodeDefinition.Network))
			if err != nil {
				log.Error(err, "failed to marshal workload definition")
				r.UnlockAndRequeue(&ethereumNodeDefinition, msg.Subject, notifPayload, requeueDelay)
				continue
			}

			// create workload definition
			jsonString := string(json)
			name := "ethereum-node"
			var companyID uint = 0
			var userID uint = 0
			workloadDefinition := v0.WorkloadDefinition{
				Definition: v0.Definition{
					Name:      &name,
					CompanyID: &companyID,
					UserID:    &userID,
				},
				JSONDocument: &jsonString,
			}


			var workloadDefinitionResponse *v0.WorkloadDefinition

			switch notif.Operation {

			case "Created":
				log.V(1).Info("received created notification")

				// persist workload definition to database
				workloadDefinitionResponse, err = client.CreateWorkloadDefinition(
					&workloadDefinition,
					r.APIServer,
					"",
				)
				if err != nil {
					log.Error(err, "failed to create workload definition")
					r.UnlockAndRequeue(&ethereumNodeDefinition, msg.Subject, notifPayload, requeueDelay)
					continue
				}
				log.V(1).Info(
					"workload definition created in API",
					"workloadDefinitionName", workloadDefinition.Name,
				)

			case "Updated":
				log.V(1).Info("received updated notification")

				workloadDefinitionResponse, err = client.UpdateWorkloadDefinition(
					&workloadDefinition,
					r.APIServer,
					"",
				)
				if err != nil {
					log.Error(err, "failed to update workload definition")
					r.UnlockAndRequeue(&ethereumNodeDefinition, msg.Subject, notifPayload, requeueDelay)
					continue
				}
				log.V(1).Info(
					"workload definition updated in API",
					"workloadDefinitionName", workloadDefinition.Name,
				)

			default:
				log.Error(err, "operation must be one of Created, Updated, or Deleted")
				continue
			}

			// set the ethereum node definition object's Reconciled field to true
			isReconciled := true
			reconciledDefinition := v0.EthereumNodeDefinition{
				Common: v0.Common{
					ID: ethereumNodeDefinition.ID,
				},
				Reconciled:           &isReconciled,
				WorkloadDefinitionID: workloadDefinitionResponse.ID,
			}

			updatedDefinition, err := client.UpdateEthereumNodeDefinition(
				&reconciledDefinition,
				r.APIServer,
				"",
			)
			if err != nil {
				log.Error(err, "failed to update ethereum node definition to mark as reconciled")
				r.UnlockAndRequeue(&ethereumNodeDefinition, msg.Subject, notifPayload, requeueDelay)
				continue
			}
			log.V(1).Info(
				"ethereum node definition marked as reconciled in API",
				"ethereumNodeDefinitionName", updatedDefinition.Name,
			)

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&ethereumNodeDefinition); !ok {
				log.V(1).Info("ethereum node definition remains locked - will unlock when TTL expires")
			} else {
				log.V(1).Info("ethereum node definition unlocked")
			}

			log.Info("ethereum node definition successfully reconciled")
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
