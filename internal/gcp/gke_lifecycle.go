package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"gorm.io/datatypes"

	notif "github.com/threeport/threeport/internal/gcp/notif"
	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	gcpauth "github.com/threeport/threeport/pkg/auth/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

// gkeLifecycle implements provider.InfraLifecycleProvider for GCP GKE
// runtime instances.
type gkeLifecycle struct {
	r          *controller.Reconciler
	instanceID uint
	instance   *v0.GcpGkeKubernetesRuntimeInstance
	log        *logr.Logger
}

// newGkeLifecycleProvider constructs an InfraLifecycleProvider for GKE.
func newGkeLifecycleProvider(
	r *controller.Reconciler,
	instance *v0.GcpGkeKubernetesRuntimeInstance,
	log *logr.Logger,
) *gkeLifecycle {
	return &gkeLifecycle{
		r:          r,
		instanceID: *instance.ID,
		instance:   instance,
		log:        log,
	}
}

// GetReconciliation fetches the latest reconciliation state from the API.
func (g *gkeLifecycle) GetReconciliation() (*provider.ReconciliationSnapshot, error) {
	latest, err := client.GetGcpGkeKubernetesRuntimeInstanceByID(
		g.r.APIClient,
		g.r.APIServer,
		g.instanceID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest GKE instance: %w", err)
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

// BuildInfra constructs the GKE infrastructure provider from API objects.
func (g *gkeLifecycle) BuildInfra() (provider.InfraProvider, error) {
	latest, err := client.GetGcpGkeKubernetesRuntimeInstanceByID(
		g.r.APIClient,
		g.r.APIServer,
		g.instanceID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get GKE instance for infra build: %w", err)
	}
	def, err := client.GetGcpGkeKubernetesRuntimeDefinitionByID(
		g.r.APIClient,
		g.r.APIServer,
		*latest.GcpGkeKubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get GKE definition: %w", err)
	}
	return buildGkeInfra(g.r, latest, def, g.log)
}

// IsCreateComplete checks whether resource inventory has been persisted.
func (g *gkeLifecycle) IsCreateComplete() (bool, error) {
	latest, err := client.GetGcpGkeKubernetesRuntimeInstanceByID(
		g.r.APIClient,
		g.r.APIServer,
		g.instanceID,
	)
	if err != nil {
		return false, fmt.Errorf("failed to check GKE creation status: %w", err)
	}
	if latest.ResourceInventory == nil {
		return false, nil
	}
	inventory := *latest.ResourceInventory
	return len(inventory) > 0 && string(inventory) != "{}" && string(inventory) != "null", nil
}

// OnCreateConfirmed gets connection info and updates the kubernetes runtime instance.
func (g *gkeLifecycle) OnCreateConfirmed(infra provider.InfraProvider) error {
	infraGKE := infra.(*provider.KubernetesRuntimeInfraGKE)
	kubeConnectionInfo, err := infraGKE.GetConnection()
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes API connection info: %w", err)
	}

	latest, err := client.GetGcpGkeKubernetesRuntimeInstanceByID(
		g.r.APIClient,
		g.r.APIServer,
		g.instanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get GKE instance for connection update: %w", err)
	}
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		g.r.APIClient,
		g.r.APIServer,
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
	if _, err = client.UpdateKubernetesRuntimeInstance(
		g.r.APIClient,
		g.r.APIServer,
		kubernetesRuntimeInstance,
	); err != nil {
		return fmt.Errorf("failed to update kubernetes runtime instance with kube connection info: %w", err)
	}
	return nil
}

// SaveCreateOutputs saves the final Pulumi state.
func (g *gkeLifecycle) SaveCreateOutputs(_ provider.InfraProvider, state *datatypes.JSON) error {
	updatedInstance := v0.GcpGkeKubernetesRuntimeInstance{
		Common:            v0.Common{ID: &g.instanceID},
		ResourceInventory: state,
	}
	if _, err := client.UpdateGcpGkeKubernetesRuntimeInstance(
		g.r.APIClient,
		g.r.APIServer,
		&updatedInstance,
	); err != nil {
		return fmt.Errorf("failed to update GKE instance with resource inventory: %w", err)
	}
	return nil
}

// OnDeleteConfirmed triggers deletion of the parent KubernetesRuntimeInstance
// if it has not already been scheduled for deletion.  This handles the case
// where a GcpGkeKubernetesRuntimeInstance is deleted directly (not via the
// KRI deletion flow), leaving the parent KRI orphaned.  In the normal KRI
// deletion flow the parent is already hard-deleted by the time this runs, so
// a not-found response is treated as a no-op.
func (g *gkeLifecycle) OnDeleteConfirmed(_ provider.InfraProvider) error {
	latest, err := client.GetGcpGkeKubernetesRuntimeInstanceByID(
		g.r.APIClient,
		g.r.APIServer,
		g.instanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get GKE instance for parent KRI cleanup: %w", err)
	}

	kri, err := client.GetKubernetesRuntimeInstanceByID(
		g.r.APIClient,
		g.r.APIServer,
		*latest.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		if errors.Is(err, client_lib.ErrObjectNotFound) {
			// KRI already deleted - normal flow where KRI deletion completes
			// before the GKE cluster destroy finishes
			return nil
		}
		return fmt.Errorf("failed to get parent KRI: %w", err)
	}

	if kri.DeletionScheduled != nil {
		// deletion already in progress via the normal KRI flow
		return nil
	}

	if _, err = client.DeleteKubernetesRuntimeInstance(
		g.r.APIClient,
		g.r.APIServer,
		*kri.ID,
	); err != nil {
		return fmt.Errorf("failed to trigger parent KRI deletion: %w", err)
	}

	return nil
}

// AckCreation sets CreationAcknowledged and clears CreationFailed.
func (g *gkeLifecycle) AckCreation() error {
	ackTimestamp := time.Now().UTC()
	creationFailed := false
	ackUpdate := v0.GcpGkeKubernetesRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			CreationAcknowledged: &ackTimestamp,
			CreationFailed:       &creationFailed,
		},
	}
	_, err := client.UpdateGcpGkeKubernetesRuntimeInstance(g.r.APIClient, g.r.APIServer, &ackUpdate)
	return err
}

// RefreshCreationAck updates CreationAcknowledged to prevent stale detection.
func (g *gkeLifecycle) RefreshCreationAck() error {
	refreshTimestamp := time.Now().UTC()
	ackUpdate := v0.GcpGkeKubernetesRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			CreationAcknowledged: &refreshTimestamp,
		},
	}
	_, err := client.UpdateGcpGkeKubernetesRuntimeInstance(g.r.APIClient, g.r.APIServer, &ackUpdate)
	return err
}

// SetCreationFailed marks CreationFailed=true in the API.
func (g *gkeLifecycle) SetCreationFailed() error {
	creationFailed := true
	failedUpdate := v0.GcpGkeKubernetesRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			CreationFailed: &creationFailed,
		},
	}
	_, err := client.UpdateGcpGkeKubernetesRuntimeInstance(g.r.APIClient, g.r.APIServer, &failedUpdate)
	return err
}

// ConfirmCreation sets CreationConfirmed and Reconciled=true.
func (g *gkeLifecycle) ConfirmCreation() error {
	reconciled := true
	timestamp := time.Now().UTC()
	confirmedUpdate := v0.GcpGkeKubernetesRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			Reconciled:        &reconciled,
			CreationConfirmed: &timestamp,
		},
	}
	_, err := client.UpdateGcpGkeKubernetesRuntimeInstance(g.r.APIClient, g.r.APIServer, &confirmedUpdate)
	return err
}

// AckDeletion sets DeletionAcknowledged in the API.
func (g *gkeLifecycle) AckDeletion() error {
	timestamp := time.Now().UTC()
	ackUpdate := v0.GcpGkeKubernetesRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			DeletionAcknowledged: &timestamp,
		},
	}
	_, err := client.UpdateGcpGkeKubernetesRuntimeInstance(g.r.APIClient, g.r.APIServer, &ackUpdate)
	return err
}

// RefreshDeletionAck updates DeletionAcknowledged to prevent stale detection.
func (g *gkeLifecycle) RefreshDeletionAck() error {
	refreshTimestamp := time.Now().UTC()
	ackUpdate := v0.GcpGkeKubernetesRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			DeletionAcknowledged: &refreshTimestamp,
		},
	}
	_, err := client.UpdateGcpGkeKubernetesRuntimeInstance(g.r.APIClient, g.r.APIServer, &ackUpdate)
	return err
}

// ConfirmDeletion sets DeletionConfirmed in the API.
func (g *gkeLifecycle) ConfirmDeletion() error {
	timestamp := time.Now().UTC()
	confirmedUpdate := v0.GcpGkeKubernetesRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			DeletionConfirmed: &timestamp,
		},
	}
	_, err := client.UpdateGcpGkeKubernetesRuntimeInstance(g.r.APIClient, g.r.APIServer, &confirmedUpdate)
	return err
}

// SaveState persists intermediate Pulumi state to the API.
func (g *gkeLifecycle) SaveState(state *datatypes.JSON) error {
	stateUpdate := v0.GcpGkeKubernetesRuntimeInstance{
		Common:            v0.Common{ID: &g.instanceID},
		ResourceInventory: state,
	}
	_, err := client.UpdateGcpGkeKubernetesRuntimeInstance(g.r.APIClient, g.r.APIServer, &stateUpdate)
	return err
}

// ClearInventory sets ResourceInventory to "{}" to signal destroy complete.
func (g *gkeLifecycle) ClearInventory() error {
	emptyInventory := datatypes.JSON([]byte("{}"))
	clearedUpdate := v0.GcpGkeKubernetesRuntimeInstance{
		Common:            v0.Common{ID: &g.instanceID},
		ResourceInventory: &emptyInventory,
	}
	_, err := client.UpdateGcpGkeKubernetesRuntimeInstance(g.r.APIClient, g.r.APIServer, &clearedUpdate)
	return err
}

// PublishCreateNotification publishes a NATS notification for creation.
func (g *gkeLifecycle) PublishCreateNotification() error {
	notifPayload, err := g.instance.NotificationPayload(
		notifications.NotificationOperationCreated,
		false,
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to create notification payload: %w", err)
	}
	if _, err = g.r.JetStreamContext.Publish(
		notif.GcpGkeKubernetesRuntimeInstanceCreateSubject,
		*notifPayload,
	); err != nil {
		return fmt.Errorf("failed to publish create notification: %w", err)
	}
	return nil
}

// PublishDeleteNotification publishes a NATS notification for deletion.
func (g *gkeLifecycle) PublishDeleteNotification() error {
	notifPayload, err := g.instance.NotificationPayload(
		notifications.NotificationOperationDeleted,
		false,
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to create notification payload: %w", err)
	}
	if _, err = g.r.JetStreamContext.Publish(
		notif.GcpGkeKubernetesRuntimeInstanceDeleteSubject,
		*notifPayload,
	); err != nil {
		return fmt.Errorf("failed to publish delete notification: %w", err)
	}
	return nil
}

// buildGkeInfra constructs a KubernetesRuntimeInfraGKE from API objects.
// requireSameProject reports whether a service account belongs to the project a
// runtime is being created in.
//
// A workload identity binding is addressed as projects/<project>/serviceAccounts
// /<account>, so an account from elsewhere is not named by that path - and the
// failure would land after the cluster's network, control plane and node pool
// are provisioned. An account whose project cannot be read from its address is
// refused for the same reason: the binding would be attempted blind.
func requireSameProject(serviceAccountEmail, projectID, providerName string) error {
	const domainSuffix = ".iam.gserviceaccount.com"

	at := strings.Index(serviceAccountEmail, "@")
	if at < 0 || !strings.HasSuffix(serviceAccountEmail, domainSuffix) {
		return fmt.Errorf(
			"the ambient service account %s is not addressable as a project service account, so the workload identity binding for GCP provider %s cannot name it",
			serviceAccountEmail,
			providerName,
		)
	}

	accountProject := strings.TrimSuffix(serviceAccountEmail[at+1:], domainSuffix)
	if accountProject != projectID {
		return fmt.Errorf(
			"this control plane runs as %s in project %s, but GCP provider %s creates runtimes in project %s: the workload identity binding is made in the runtime's project and cannot name an account from another one",
			serviceAccountEmail,
			accountProject,
			providerName,
			projectID,
		)
	}

	return nil
}

func buildGkeInfra(
	r *controller.Reconciler,
	instance *v0.GcpGkeKubernetesRuntimeInstance,
	definition *v0.GcpGkeKubernetesRuntimeDefinition,
	log *logr.Logger,
) (*provider.KubernetesRuntimeInfraGKE, error) {
	gcpProvider, err := client.GetGcpProviderByID(
		r.APIClient,
		r.APIServer,
		*instance.GcpProviderID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve GCP provider by ID: %w", err)
	}

	infraGKE := &provider.KubernetesRuntimeInfraGKE{
		PulumiWorkspace: provider.PulumiWorkspace{
			RuntimeInstanceName: *instance.Name,
			Logger:              log,
		},
		ProjectID:              *gcpProvider.ProjectID,
		Region:                 *instance.Region,
		WorkerNodeInitialCount: int32(*definition.DefaultNodeGroupInitialSize),
		MachineType:            *definition.DefaultNodeGroupInstanceType,
		MinNodeCount:           int32(*definition.DefaultNodeGroupMinimumSize),
		MaxNodeCount:           int32(*definition.DefaultNodeGroupMaximumSize),
	}

	// The workload identity binding made after the cluster is created names this
	// account, so it has to be known before any of the cluster is provisioned -
	// not discovered missing once the network, control plane and node pool are
	// already up.
	if gcpProvider.ServiceAccountCredentials != nil && *gcpProvider.ServiceAccountCredentials != "" {
		infraGKE.ServiceAccountCredentials = *gcpProvider.ServiceAccountCredentials
		email, err := serviceAccountEmailFromCredentials(infraGKE.ServiceAccountCredentials)
		if err != nil {
			return nil, fmt.Errorf("failed to extract service account email: %w", err)
		}
		infraGKE.ServiceAccountEmail = email
	} else {
		// No stored key: this controller authenticates with the ambient identity
		// GCP gives it, and that identity is the account to bind.
		email, err := gcpauth.AmbientServiceAccountEmail(context.Background())
		if err != nil {
			return nil, fmt.Errorf(
				"GCP provider %s stores no service account credentials and no ambient service account could be resolved, so the workload identity binding has no account to name: %w",
				*gcpProvider.Name,
				err,
			)
		}

		// The binding addresses the account through the cluster's project. An
		// ambient identity belonging to another project is not reachable at that
		// path, and the binding would fail once the cluster already exists.
		if err := requireSameProject(email, infraGKE.ProjectID, *gcpProvider.Name); err != nil {
			return nil, err
		}

		infraGKE.ServiceAccountEmail = email
	}

	return infraGKE, nil
}

// serviceAccountEmailFromCredentials extracts the client_email field from a
// GCP service account key JSON blob.
func serviceAccountEmailFromCredentials(credentialsJSON string) (string, error) {
	var key struct {
		ClientEmail string `json:"client_email"`
	}
	if err := json.Unmarshal([]byte(credentialsJSON), &key); err != nil {
		return "", fmt.Errorf("failed to parse service account credentials JSON: %w", err)
	}
	if key.ClientEmail == "" {
		return "", fmt.Errorf("service account credentials JSON has no client_email field")
	}
	return key.ClientEmail, nil
}
