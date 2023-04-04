package reconcile

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/mitchellh/mapstructure"
	yamlv3 "gopkg.in/yaml.v3"
	"gorm.io/datatypes"

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
			var notif notifications.Notification
			if err := json.Unmarshal(msg.Data, &notif); err != nil {
				log.Error(
					err, "failed to unmarshal message data from NATS",
					"msgSubject", msg.Subject,
					"msgData", string(msg.Data),
				)
				go r.RequeueRaw(msg.Subject, msg.Data)
				log.V(1).Info("ethereum node definition reconciliation requeued with identical payload and fixed delay")
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

			// // iterate over each resource in the yaml doc and construct a workload
			// // resource definition
			// decoder := yamlv3.NewDecoder(strings.NewReader(*ethereumNodeDefinition.YAMLDocument))
			// var workloadResourceDefinitions []v0.WorkloadResourceDefinition
			// wrdConstructSuccess := true
			// for {
			// 	// decode the next resource, exit loop if the end has been reached
			// 	var node yamlv3.Node
			// 	err := decoder.Decode(&node)
			// 	if errors.Is(err, io.EOF) {
			// 		break
			// 	}
			// 	if err != nil {
			// 		log.Error(err, "failed to decode yaml node in ethereum node definition")
			// 		wrdConstructSuccess = false
			// 		break
			// 	}

			// 	// marshal the yaml
			// 	yamlContent, err := yamlv3.Marshal(&node)
			// 	if err != nil {
			// 		log.Error(err, "failed to marshal yaml from ethereum node definition")
			// 		wrdConstructSuccess = false
			// 		break
			// 	}

			// 	// convert yaml to json
			// 	jsonContent, err := yaml.YAMLToJSON(yamlContent)
			// 	if err != nil {
			// 		log.Error(err, "failed to convert yaml to json")
			// 		wrdConstructSuccess = false
			// 		break
			// 	}

			// 	// unmarshal the json into the type used by API
			// 	var jsonDefinition datatypes.JSON
			// 	if err := jsonDefinition.UnmarshalJSON(jsonContent); err != nil {
			// 		log.Error(err, "failed to unmarshal json to datatypes.JSON")
			// 		wrdConstructSuccess = false
			// 		break
			// 	}

			// 	// build the workload resource definition and marshal to json
			// 	workloadResourceDefinition := v0.WorkloadResourceDefinition{
			// 		JSONDefinition:           &jsonDefinition,
			// 		EthereumNodeDefinitionID: ethereumNodeDefinition.ID,
			// 	}
			// 	workloadResourceDefinitions = append(workloadResourceDefinitions, workloadResourceDefinition)
			// }

			// // if any workload resource definitions failed construction, abort
			// if !wrdConstructSuccess {
			// 	log.Error(err, "failed to construct workload resource definition objects")
			// 	r.UnlockAndRequeue(&ethereumNodeDefinition, msg.Subject, notifPayload, requeueDelay)
			// 	continue
			// }

			// // marshal the workload resource definitions to JSON for creation in API
			// wrdsJSON, err := json.Marshal(&workloadResourceDefinitions)
			// if err != nil {
			// 	log.Error(err, "failed to marshal workload resource definitions to json")
			// 	r.UnlockAndRequeue(&ethereumNodeDefinition, msg.Subject, notifPayload, requeueDelay)
			// 	continue
			// }

			// // create workload resource definitions in API
			// wrds, err := client.CreateWorkloadResourceDefinitions(
			// 	wrdsJSON,
			// 	r.APIServer,
			// 	"",
			// )
			// if err != nil {
			// 	log.Error(err, "failed to create workload resource definitions in API")
			// 	r.UnlockAndRequeue(&ethereumNodeDefinition, msg.Subject, notifPayload, requeueDelay)
			// 	continue
			// }
			// for _, wrd := range *wrds {
			// 	log.V(1).Info(
			// 		"workload resource definition created",
			// 		"workloadResourceDefinitionID", wrd.ID,
			// 	)
			// }

			// set the object's Reconciled field to true
			// wdReconciled := true
			// reconciledWD := v0.EthereumNodeDefinition{
			// 	Common: v0.Common{
			// 		ID: ethereumNodeDefinition.ID,
			// 	},
			// 	Reconciled: &wdReconciled,
			// }
			//reconciledWDJSON, err := json.Marshal(&reconciledWD)
			//if err != nil {
			//	log.Error(err, "failed to marshal json for ethereum node definition update to mark as reconciled")
			//	r.UnlockAndRequeue(&ethereumNodeDefinition, msg.Subject, notifPayload, requeueDelay)
			//	continue
			//}
			updatedWD, err := client.UpdateEthereumNodeDefinition(
				&reconciledWD,
				//*ethereumNodeDefinition.ID,
				//reconciledWDJSON,
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
				"ethereumNodeDefinitionName", updatedWD.Name,
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
