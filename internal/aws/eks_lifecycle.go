package aws

import (
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"gorm.io/datatypes"

	notif "github.com/threeport/threeport/internal/aws/notif"
	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	tpaws "github.com/threeport/threeport/pkg/aws/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

// eksLifecycle implements provider.InfraLifecycleProvider for AWS EKS runtime
// instances.
type eksLifecycle struct {
	r          *controller.Reconciler
	instanceID uint
	instance   *v0.AwsEksKubernetesRuntimeInstance
	log        *logr.Logger
}

// newEksLifecycleProvider constructs an InfraLifecycleProvider for EKS.
func newEksLifecycleProvider(
	r *controller.Reconciler,
	instance *v0.AwsEksKubernetesRuntimeInstance,
	log *logr.Logger,
) *eksLifecycle {
	return &eksLifecycle{
		r:          r,
		instanceID: *instance.ID,
		instance:   instance,
		log:        log,
	}
}

// GetReconciliation fetches the latest reconciliation state from the API.
func (e *eksLifecycle) GetReconciliation() (*provider.ReconciliationSnapshot, error) {
	latest, err := client.GetAwsEksKubernetesRuntimeInstanceByID(
		e.r.APIClient,
		e.r.APIServer,
		e.instanceID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest EKS instance: %w", err)
	}

	creationFailed := false
	if latest.CreationFailed != nil {
		creationFailed = *latest.CreationFailed
	}

	return &provider.ReconciliationSnapshot{
		CreationAcknowledged: latest.CreationAcknowledged,
		CreationConfirmed:    latest.CreationConfirmed,
		CreationFailed:       creationFailed,
		DeletionScheduled:    latest.DeletionScheduled,
		DeletionAcknowledged: latest.DeletionAcknowledged,
		DeletionConfirmed:    latest.DeletionConfirmed,
		ResourceInventory:    latest.ResourceInventory,
	}, nil
}

// BuildInfra constructs the EKS infrastructure provider from API objects.
func (e *eksLifecycle) BuildInfra() (provider.InfraProvider, error) {
	latest, err := client.GetAwsEksKubernetesRuntimeInstanceByID(
		e.r.APIClient,
		e.r.APIServer,
		e.instanceID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get EKS instance for infra build: %w", err)
	}
	definition, err := client.GetAwsEksKubernetesRuntimeDefinitionByID(
		e.r.APIClient,
		e.r.APIServer,
		*latest.AwsEksKubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get EKS definition: %w", err)
	}

	return buildEksInfra(e.r, latest, definition, e.log)
}

// IsCreateComplete reports whether the whole create finished, not just the
// Pulumi stack.
//
// Stack state is streamed to ResourceInventory while the stack is still being
// built, so a non-empty inventory only says that provisioning started.  A
// create interrupted after the stack but before the IRSA roles and the EBS CSI
// addon would otherwise be confirmed, leaving a cluster that looks ready while
// none of its add-ons can assume an IAM role.
//
// The addon is the last thing CreateInfra does, so its presence stands for
// everything before it.
func (e *eksLifecycle) IsCreateComplete() (bool, error) {
	latest, err := client.GetAwsEksKubernetesRuntimeInstanceByID(
		e.r.APIClient,
		e.r.APIServer,
		e.instanceID,
	)
	if err != nil {
		return false, fmt.Errorf("failed to check EKS creation status: %w", err)
	}
	if latest.ResourceInventory == nil {
		return false, nil
	}
	inventory := *latest.ResourceInventory
	if len(inventory) == 0 || string(inventory) == "{}" || string(inventory) == "null" {
		return false, nil
	}

	infra, err := e.BuildInfra()
	if err != nil {
		return false, fmt.Errorf("failed to build infra to check EKS creation status: %w", err)
	}
	infraEKS := infra.(*provider.KubernetesRuntimeInfraEKS)

	complete, err := tpaws.EksStorageAddonActive(infraEKS.AwsConfig, infraEKS.RuntimeInstanceName)
	if err != nil {
		return false, fmt.Errorf("failed to check whether the EKS storage addon is active: %w", err)
	}

	return complete, nil
}

// OnCreateConfirmed gets connection info and updates the kubernetes runtime
// instance with it.
func (e *eksLifecycle) OnCreateConfirmed(infra provider.InfraProvider) error {
	infraEKS := infra.(*provider.KubernetesRuntimeInfraEKS)
	kubeConnectionInfo, err := infraEKS.GetConnection()
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes API connection info: %w", err)
	}

	latest, err := client.GetAwsEksKubernetesRuntimeInstanceByID(
		e.r.APIClient,
		e.r.APIServer,
		e.instanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get EKS instance for connection update: %w", err)
	}
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		e.r.APIClient,
		e.r.APIServer,
		*latest.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance: %w", err)
	}

	kubeRuntimeReconciled := false
	kubernetesRuntimeInstance.APIEndpoint = &kubeConnectionInfo.APIEndpoint
	kubernetesRuntimeInstance.CACertificate = &kubeConnectionInfo.CACertificate
	kubernetesRuntimeInstance.ConnectionToken = &kubeConnectionInfo.Token
	kubernetesRuntimeInstance.ConnectionTokenExpiration = &kubeConnectionInfo.TokenExpiration
	kubernetesRuntimeInstance.Reconciled = &kubeRuntimeReconciled
	if _, err := client.UpdateKubernetesRuntimeInstance(
		e.r.APIClient,
		e.r.APIServer,
		kubernetesRuntimeInstance,
	); err != nil {
		return fmt.Errorf("failed to update kubernetes runtime instance with kube connection info: %w", err)
	}

	return nil
}

// SaveCreateOutputs saves the final Pulumi state.
func (e *eksLifecycle) SaveCreateOutputs(_ provider.InfraProvider, state *datatypes.JSON) error {
	updatedInstance := v0.AwsEksKubernetesRuntimeInstance{
		Common:            v0.Common{ID: &e.instanceID},
		ResourceInventory: state,
	}
	if _, err := client.UpdateAwsEksKubernetesRuntimeInstance(
		e.r.APIClient,
		e.r.APIServer,
		&updatedInstance,
	); err != nil {
		return fmt.Errorf("failed to update EKS instance with resource inventory: %w", err)
	}

	return nil
}

// OnDeleteConfirmed triggers deletion of the parent KubernetesRuntimeInstance
// if it has not already been scheduled for deletion.  This handles the case
// where an AwsEksKubernetesRuntimeInstance is deleted directly rather than via
// the KRI deletion flow, which would otherwise leave the parent KRI orphaned.
// In the normal flow the parent is already hard-deleted by the time this runs,
// so a not-found response is a no-op.  GKE and OKE both do this; the
// hand-rolled AWS reconciler did not.
func (e *eksLifecycle) OnDeleteConfirmed(_ provider.InfraProvider) error {
	latest, err := client.GetAwsEksKubernetesRuntimeInstanceByID(
		e.r.APIClient,
		e.r.APIServer,
		e.instanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get EKS instance for parent KRI cleanup: %w", err)
	}
	if latest.KubernetesRuntimeInstanceID == nil {
		return nil
	}

	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		e.r.APIClient,
		e.r.APIServer,
		*latest.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		if errors.Is(err, client_lib.ErrObjectNotFound) {
			// the KRI deletion completed before the cluster destroy finished,
			// which is the normal flow
			return nil
		}
		return fmt.Errorf("failed to get parent KRI: %w", err)
	}

	if kubernetesRuntimeInstance.DeletionScheduled != nil {
		// deletion already in progress via the normal KRI flow
		return nil
	}

	if _, err := client.DeleteKubernetesRuntimeInstance(
		e.r.APIClient,
		e.r.APIServer,
		*kubernetesRuntimeInstance.ID,
	); err != nil {
		return fmt.Errorf("failed to trigger parent KRI deletion: %w", err)
	}

	return nil
}

// AckCreation sets CreationAcknowledged and clears CreationFailed.
func (e *eksLifecycle) AckCreation() error {
	ackTimestamp := time.Now().UTC()
	creationFailed := false
	ackUpdate := v0.AwsEksKubernetesRuntimeInstance{
		Common: v0.Common{ID: &e.instanceID},
		Reconciliation: v0.Reconciliation{
			CreationAcknowledged: &ackTimestamp,
			CreationFailed:       &creationFailed,
		},
	}
	_, err := client.UpdateAwsEksKubernetesRuntimeInstance(e.r.APIClient, e.r.APIServer, &ackUpdate)

	return err
}

// RefreshCreationAck updates CreationAcknowledged to prevent stale detection.
func (e *eksLifecycle) RefreshCreationAck() error {
	refreshTimestamp := time.Now().UTC()
	ackUpdate := v0.AwsEksKubernetesRuntimeInstance{
		Common: v0.Common{ID: &e.instanceID},
		Reconciliation: v0.Reconciliation{
			CreationAcknowledged: &refreshTimestamp,
		},
	}
	_, err := client.UpdateAwsEksKubernetesRuntimeInstance(e.r.APIClient, e.r.APIServer, &ackUpdate)

	return err
}

// SetCreationFailed marks CreationFailed=true in the API.
func (e *eksLifecycle) SetCreationFailed() error {
	creationFailed := true
	failedUpdate := v0.AwsEksKubernetesRuntimeInstance{
		Common: v0.Common{ID: &e.instanceID},
		Reconciliation: v0.Reconciliation{
			CreationFailed: &creationFailed,
		},
	}
	_, err := client.UpdateAwsEksKubernetesRuntimeInstance(e.r.APIClient, e.r.APIServer, &failedUpdate)

	return err
}

// ConfirmCreation sets CreationConfirmed and Reconciled=true.
func (e *eksLifecycle) ConfirmCreation() error {
	reconciled := true
	timestamp := time.Now().UTC()
	confirmedUpdate := v0.AwsEksKubernetesRuntimeInstance{
		Common: v0.Common{ID: &e.instanceID},
		Reconciliation: v0.Reconciliation{
			Reconciled:        &reconciled,
			CreationConfirmed: &timestamp,
		},
	}
	_, err := client.UpdateAwsEksKubernetesRuntimeInstance(e.r.APIClient, e.r.APIServer, &confirmedUpdate)

	return err
}

// AckDeletion sets DeletionAcknowledged in the API.
func (e *eksLifecycle) AckDeletion() error {
	timestamp := time.Now().UTC()
	ackUpdate := v0.AwsEksKubernetesRuntimeInstance{
		Common: v0.Common{ID: &e.instanceID},
		Reconciliation: v0.Reconciliation{
			DeletionAcknowledged: &timestamp,
		},
	}
	_, err := client.UpdateAwsEksKubernetesRuntimeInstance(e.r.APIClient, e.r.APIServer, &ackUpdate)

	return err
}

// RefreshDeletionAck updates DeletionAcknowledged to prevent stale detection.
func (e *eksLifecycle) RefreshDeletionAck() error {
	refreshTimestamp := time.Now().UTC()
	ackUpdate := v0.AwsEksKubernetesRuntimeInstance{
		Common: v0.Common{ID: &e.instanceID},
		Reconciliation: v0.Reconciliation{
			DeletionAcknowledged: &refreshTimestamp,
		},
	}
	_, err := client.UpdateAwsEksKubernetesRuntimeInstance(e.r.APIClient, e.r.APIServer, &ackUpdate)

	return err
}

// ConfirmDeletion sets DeletionConfirmed in the API.
func (e *eksLifecycle) ConfirmDeletion() error {
	timestamp := time.Now().UTC()
	confirmedUpdate := v0.AwsEksKubernetesRuntimeInstance{
		Common: v0.Common{ID: &e.instanceID},
		Reconciliation: v0.Reconciliation{
			DeletionConfirmed: &timestamp,
		},
	}
	_, err := client.UpdateAwsEksKubernetesRuntimeInstance(e.r.APIClient, e.r.APIServer, &confirmedUpdate)

	return err
}

// SaveState persists intermediate Pulumi state to the API.
func (e *eksLifecycle) SaveState(state *datatypes.JSON) error {
	stateUpdate := v0.AwsEksKubernetesRuntimeInstance{
		Common:            v0.Common{ID: &e.instanceID},
		ResourceInventory: state,
	}
	_, err := client.UpdateAwsEksKubernetesRuntimeInstance(e.r.APIClient, e.r.APIServer, &stateUpdate)

	return err
}

// ClearInventory sets ResourceInventory to "{}" to signal destroy complete.
func (e *eksLifecycle) ClearInventory() error {
	emptyInventory := datatypes.JSON([]byte("{}"))
	clearedUpdate := v0.AwsEksKubernetesRuntimeInstance{
		Common:            v0.Common{ID: &e.instanceID},
		ResourceInventory: &emptyInventory,
	}
	_, err := client.UpdateAwsEksKubernetesRuntimeInstance(e.r.APIClient, e.r.APIServer, &clearedUpdate)

	return err
}

// PublishCreateNotification publishes a NATS notification for creation.
func (e *eksLifecycle) PublishCreateNotification() error {
	notifPayload, err := e.instance.NotificationPayload(
		notifications.NotificationOperationCreated,
		false,
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to create notification payload: %w", err)
	}
	if _, err := e.r.JetStreamContext.Publish(
		notif.AwsEksKubernetesRuntimeInstanceCreateSubject,
		*notifPayload,
	); err != nil {
		return fmt.Errorf("failed to publish create notification: %w", err)
	}

	return nil
}

// PublishDeleteNotification publishes a NATS notification for deletion.
func (e *eksLifecycle) PublishDeleteNotification() error {
	notifPayload, err := e.instance.NotificationPayload(
		notifications.NotificationOperationDeleted,
		false,
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to create notification payload: %w", err)
	}
	if _, err := e.r.JetStreamContext.Publish(
		notif.AwsEksKubernetesRuntimeInstanceDeleteSubject,
		*notifPayload,
	); err != nil {
		return fmt.Errorf("failed to publish delete notification: %w", err)
	}

	return nil
}

// buildEksInfra constructs a KubernetesRuntimeInfraEKS from API objects.
func buildEksInfra(
	r *controller.Reconciler,
	instance *v0.AwsEksKubernetesRuntimeInstance,
	definition *v0.AwsEksKubernetesRuntimeDefinition,
	log *logr.Logger,
) (*provider.KubernetesRuntimeInfraEKS, error) {
	awsProvider, err := client.GetAwsProviderByID(
		r.APIClient,
		r.APIServer,
		*instance.AwsProviderID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve AWS provider by ID: %w", err)
	}

	awsConfig, err := kube.GetAwsConfigFromAwsProvider(
		r.EncryptionKey,
		*instance.Region,
		awsProvider,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS config: %w", err)
	}

	providerCredentials, err := kube.GetAwsProviderCredentialsFromAwsProvider(
		r.EncryptionKey,
		awsProvider,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve AWS provider credentials: %w", err)
	}

	return eksInfraFromApiObjects(
		instance,
		definition,
		*awsProvider.AccountID,
		awsConfig,
		*providerCredentials,
		log,
	), nil
}

// eksInfraFromApiObjects maps the runtime instance and definition onto the
// infra object the Pulumi program provisions from.
func eksInfraFromApiObjects(
	instance *v0.AwsEksKubernetesRuntimeInstance,
	definition *v0.AwsEksKubernetesRuntimeDefinition,
	awsAccountId string,
	awsConfig *aws.Config,
	providerCredentials tpaws.AwsProviderCredentials,
	log *logr.Logger,
) *provider.KubernetesRuntimeInfraEKS {
	return &provider.KubernetesRuntimeInfraEKS{
		PulumiWorkspace: provider.PulumiWorkspace{
			RuntimeInstanceName: *instance.Name,
			Logger:              log,
		},
		AwsAccountID:                 awsAccountId,
		AwsConfig:                    awsConfig,
		ProviderCredentials:          providerCredentials,
		Region:                       *instance.Region,
		ZoneCount:                    int32(*definition.ZoneCount),
		DefaultNodeGroupInstanceType: *definition.DefaultNodeGroupInstanceType,
		DefaultNodeGroupInitialNodes: int32(*definition.DefaultNodeGroupInitialSize),
		DefaultNodeGroupMinNodes:     int32(*definition.DefaultNodeGroupMinimumSize),
		DefaultNodeGroupMaxNodes:     int32(*definition.DefaultNodeGroupMaximumSize),
	}
}
