package reconcile

import (
	"encoding/json"
	"strings"

	"github.com/mitchellh/mapstructure"
	yamlv3 "gopkg.in/yaml.v3"
	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/controller"
	"github.com/threeport/threeport/pkg/notifications"
)

// WorkloadDefinitionReconciler reconciles system state when a WorkloadDefinition
// is created, updated or deleted.
func WorkloadDefinitionReconciler(r *controller.Reconciler) {
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
				log.V(1).Info("workload definition reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// decode the object that was created
			var workloadDefinition v0.WorkloadDefinition
			mapstructure.Decode(notif.Object, &workloadDefinition)
			log = log.WithValues("workloadDefinitionID", workloadDefinition.ID)

			// check if the object has been reconciled
			if *workloadDefinition.Reconciled {
				log.V(1).Info("workload definition has already been reconciled, ignoring notification")
				continue
			}

			// back off the requeue delay as needed
			requeueDelay := controller.SetRequeueDelay(
				notif.LastRequeueDelay,
				controller.DefaultInitialRequeueDelay,
				controller.DefaultMaxRequeueDelay,
			)

			// build the notif payload for requeues
			notifPayload, err := workloadDefinition.NotificationPayload(true, requeueDelay, notif.Operation)
			if err != nil {
				log.Error(err, "failed to build notification payload for requeue")
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("workload definition reconciliation requeued with identical payload and fixed delay")
				continue
			}

			// check for lock on object
			locked, ok := r.CheckLock(&workloadDefinition)
			if locked || !ok {
				go r.Requeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("workload definition reconciliation requeued")
				continue
			}

			// put a lock on the reconciliation of the created object
			if ok := r.Lock(&workloadDefinition); !ok {
				go r.Requeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
				log.V(1).Info("workload definition reconciliation requeued")
				continue
			}

			var workloadResourceDefinitions []v0.WorkloadResourceDefinition

			// check for deletion
			if notif.Operation == "Deleted" {
				log.V(1).Info("received deleted notification")

				_, err = client.DeleteWorkloadResourceDefinitions(
					&workloadResourceDefinitions,
					r.APIServer,
					"",
				)
				if err != nil {
					log.Error(err, "failed to delete workload resource definitions in API")
					r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
					continue
				}

				log.V(1).Info(
					"workload resource definitions deleted in API",
					"workloadDefinitionName", workloadDefinition.Name,
				)
			}

			// retrieve latest version of object if requeued
			if notif.Requeue {
				latestWorkloadDefinition, err := client.GetWorkloadDefinitionByID(
					*workloadDefinition.ID,
					r.APIServer,
					"",
				)
				if err != nil {
					log.Error(err, "failed to get workload definition by ID from API")
					r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
					continue
				}
				workloadDefinition = *latestWorkloadDefinition
			}

			// iterate over each resource in the json doc and construct a workload
			// resource definition

			var jsonObjects []map[string]interface{}
			err = yamlv3.Unmarshal([]byte(*workloadDefinition.JSONDocument), &jsonObjects)
			if err != nil {
				log.Error(err, "failed to unmarshal json document")
				r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
				continue
			}

			wrdConstructSuccess := true
			for _, jsonObject := range jsonObjects {

				// unmarshal the json into the type used by API
				bytes, _ := json.Marshal(jsonObject)
				var jsonDefinition datatypes.JSON
				if err := jsonDefinition.UnmarshalJSON(bytes); err != nil {
					log.Error(err, "failed to unmarshal json to datatypes.JSON")
					wrdConstructSuccess = false
					break
				}

				// build the workload resource definition and marshal to json
				workloadResourceDefinition := v0.WorkloadResourceDefinition{
					JSONDefinition:       &jsonDefinition,
					WorkloadDefinitionID: workloadDefinition.ID,
				}
				workloadResourceDefinitions = append(workloadResourceDefinitions, workloadResourceDefinition)
			}

			// if any workload resource definitions failed construction, abort
			if !wrdConstructSuccess {
				log.Error(err, "failed to construct workload resource definition objects")
				r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
				continue
			}

			var wrds *[]v0.WorkloadResourceDefinition

			// create or update workload resource definitions in API
			switch notif.Operation {

			case "Created":
				log.V(1).Info("received created notification")

				wrds, err = client.CreateWorkloadResourceDefinitions(
					&workloadResourceDefinitions,
					r.APIServer,
					"",
				)
				if err != nil {
					log.Error(err, "failed to create workload resource definitions in API")
					r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
					continue
				}

				log.V(1).Info(
					"workload resource definitions created in API",
					"workloadDefinitionName", workloadDefinition.Name,
				)

			case "Updated":
				log.V(1).Info("received updated notification")

				wrds, err = client.UpdateWorkloadResourceDefinitions(
					&workloadResourceDefinitions,
					r.APIServer,
					"",
				)
				if err != nil {
					log.Error(err, "failed to update workload resource definitions in API")
					r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
					continue
				}

				log.V(1).Info(
					"workload resource definitions updated in API",
					"workloadDefinitionName", workloadDefinition.Name,
				)

			default:
				log.Error(err, "operation must be one of Created, Updated, or Deleted")
				continue
			}

			for _, wrd := range *wrds {
				log.V(1).Info(
					"workload resource definitions "+strings.ToLower(notif.Operation),
					"workloadResourceDefinitionID", wrd.ID,
				)
			}

			// set the object's Reconciled field to true
			wdReconciled := true
			reconciledWD := v0.WorkloadDefinition{
				Common: v0.Common{
					ID: workloadDefinition.ID,
				},
				Reconciled: &wdReconciled,
			}
			updatedWD, err := client.UpdateWorkloadDefinition(
				&reconciledWD,
				r.APIServer,
				"",
			)
			if err != nil {
				log.Error(err, "failed to update workload definition to mark as reconciled")
				r.UnlockAndRequeue(&workloadDefinition, msg.Subject, notifPayload, requeueDelay)
				continue
			}
			log.V(1).Info(
				"workload definition marked as reconciled in API",
				"workloadDefinitionName", updatedWD.Name,
			)

			// release the lock on the reconciliation of the created object
			if ok := r.ReleaseLock(&workloadDefinition); !ok {
				log.V(1).Info("workload definition remains locked - will unlock when TTL expires")
			} else {
				log.V(1).Info("workload definition unlocked")
			}

			log.Info("workload definition successfully reconciled")
		}
	}

	r.Sub.Unsubscribe()
	reconcilerLog.Info("reconciler shutting down")
	r.ShutdownWait.Done()
}
