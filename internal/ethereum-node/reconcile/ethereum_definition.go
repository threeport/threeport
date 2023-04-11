package reconcile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"

	"github.com/mitchellh/mapstructure"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/controller"
	"github.com/threeport/threeport/pkg/notifications"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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

			// generate random 32 byte hex string
			b := make([]byte, 32)
			_, err = rand.Read(b)
			if err != nil {
				panic(err)
			}

			// Convert the byte slice to a hex string
			hexString := hex.EncodeToString(b)

			// Create auth jwt secret for consensus client -> execution client auth
			var authJWT = &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "auth-jwt",
						"namespace": "default",
					},
					"stringData": map[string]interface{}{
						"secret": hexString,
					},
				},
			}

			// define execution client manifest
			var executionClient = &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "ethereum.kotal.io/v1alpha1",
					"kind":       "Node",
					"metadata": map[string]interface{}{
						"name":      "ethereum-node-execution",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"image":         "ethereum/client-go:v1.11.5",
						"client":        "geth",
						"network":       *ethereumNodeDefinition.Network,
						"rpc":           true,
						"jwtSecretName": "auth-jwt",
						"engine":        true,
						"enginePort":    8551,
						"resources": map[string]interface{}{
							"cpu":         "2",
							"cpuLimit":    "4",
							"memory":      "8Gi",
							"memoryLimit": "16Gi",
						},
					},
				},
			}

			// define consensus client manifest
			var consensusClient = &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "ethereum2.kotal.io/v1alpha1",
					"kind":       "BeaconNode",
					"metadata": map[string]interface{}{
						"name":      "ethereum-node-consensus",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"image":                   "prysmaticlabs/prysm-beacon-chain:v4.0.1",
						"client":                  "prysm",
						"network":                 *ethereumNodeDefinition.Network,
						"rpc":                     true,
						"jwtSecretName":           "auth-jwt",
						"executionEngineEndpoint": "http://ethereum-node.default.svc.cluster.local:8551",
						"checkpointSyncUrl":       "https://prater-checkpoint-sync.stakely.io/",
						"resources": map[string]interface{}{
							"cpu":         "2",
							"cpuLimit":    "4",
							"memory":      "8Gi",
							"memoryLimit": "16Gi",
						},
					},
				},
			}

			// aggregate manifests into a single yaml
			var objects []runtime.Object
			objects = append(
								objects,
								CreateManifestMutatingWebhookConfiguration(),
								CreateManifestValidatingWebhookConfiguration(),
								CreateManifestNamespace(),
								CreateCRDNodesEthereumKotalIo(),
								CreateCRDBeaconnodesEthereum2KotalIo(),
								CreateCRDAptosKotalIo(),
								CreateCRDBitcoinKotalIo(),
								CreateCRDChainlinkKotalIo(),
								CreateCRDIpfsKotalIo(),
								CreateCRDIpfsPeerKotalIo(),
								CreateCRDFilecoinKotalIo(),
								CreateCRDNearKotalIo(),
								CreateCRDGraphKotalIo(),
								CreateCRDPolkadotKotalIo(),
								CreateCRDStacksKotalIo(),
								CreateCRDValidatorKotalIo(),
								CreateManifestClusterRole(),
								CreateManifestClusterRoleMetricsReader(),
								CreateManifestClusterRoleProxyRole(),
								CreateManifestClusterRoleBindingManager(),
								CreateManifestClusterRoleBindingProxy(),
								CreateManifestRoleBindingLeaderElection(),
								CreateManifestRole(),
								CreateManifestServiceMetrics(),
								CreateManifestServiceWebhook(),
								CreateManifestServiceAccount(),
								CreateManifestDeploymentControllerManager(),
								CreateManifestCertificate(),
								CreateManifestIssuer(),
								authJWT,
								executionClient,
								consensusClient,
							)

			json, err := json.Marshal(objects)
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

			// persist workload definition to database
			workloadDefinitionResponse, err := client.CreateWorkloadDefinition(
				&workloadDefinition,
				r.APIServer,
				"",
			)
			if err != nil {
				log.Error(err, "failed to create workload definition")
				r.UnlockAndRequeue(&ethereumNodeDefinition, msg.Subject, notifPayload, requeueDelay)
				continue
			}

			// set the ethereum node definition object's Reconciled field to true
			isReconciled := true
			reconciledDefinition := v0.EthereumNodeDefinition{
				Common: v0.Common{
					ID: ethereumNodeDefinition.ID,
				},
				Reconciled: &isReconciled,
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
