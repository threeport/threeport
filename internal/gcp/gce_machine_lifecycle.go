package gcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"gorm.io/datatypes"

	notif "github.com/threeport/threeport/internal/gcp/notif"
	"github.com/threeport/threeport/internal/provider"
	machine "github.com/threeport/threeport/internal/provider/machine"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	encryption "github.com/threeport/threeport/pkg/encryption/v0"
	event "github.com/threeport/threeport/pkg/event/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// defaultGceImageID is the boot image used when a GCE machine runtime
// definition leaves ImageID unset.
const defaultGceImageID = "debian-cloud/debian-12"

// gceMachineLifecycle implements provider.InfraLifecycleProvider for GCP GCE
// machine runtime instances.
type gceMachineLifecycle struct {
	r          *controller.Reconciler
	instanceID uint
	instance   *v0.GcpGceMachineRuntimeInstance
	log        *logr.Logger
}

var _ provider.InfraLifecycleProvider = (*gceMachineLifecycle)(nil)

// StackKey returns the runtime-instance name so the shared state machine
// serializes pulumi operations against one local state directory.
func (g *gceMachineLifecycle) StackKey() string {
	if g.instance == nil || g.instance.Name == nil {
		if g.log != nil {
			g.log.Info("GCE machine runtime instance missing name; returning empty stack key")
		}
		return ""
	}
	return *g.instance.Name
}

// newGceMachineLifecycleProvider constructs an InfraLifecycleProvider for a GCE
// machine runtime instance.
func newGceMachineLifecycleProvider(
	r *controller.Reconciler,
	instance *v0.GcpGceMachineRuntimeInstance,
	log *logr.Logger,
) *gceMachineLifecycle {
	return &gceMachineLifecycle{
		r:          r,
		instanceID: *instance.ID,
		instance:   instance,
		log:        log,
	}
}

// GetReconciliation fetches the latest reconciliation state from the API.
func (g *gceMachineLifecycle) GetReconciliation() (*provider.ReconciliationSnapshot, error) {
	latest, err := client.GetGcpGceMachineRuntimeInstanceByID(
		g.r.APIClient,
		g.r.APIServer,
		g.instanceID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest GCE instance: %w", err)
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

// BuildInfra constructs the GCE infrastructure provider from API objects.
func (g *gceMachineLifecycle) BuildInfra() (provider.InfraProvider, error) {
	latest, err := client.GetGcpGceMachineRuntimeInstanceByID(
		g.r.APIClient,
		g.r.APIServer,
		g.instanceID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get GCE instance for infra build: %w", err)
	}
	return buildGceMachineInfra(g.r, latest, g.log)
}

// IsCreateComplete reports whether persisted resource inventory is non-empty
// after trimming whitespace and rejecting placeholder values.
func (g *gceMachineLifecycle) IsCreateComplete() (bool, error) {
	latest, err := client.GetGcpGceMachineRuntimeInstanceByID(
		g.r.APIClient,
		g.r.APIServer,
		g.instanceID,
	)
	if err != nil {
		return false, fmt.Errorf("failed to check GCE creation status: %w", err)
	}
	if latest.ResourceInventory == nil {
		return false, nil
	}
	inventory := strings.TrimSpace(string(*latest.ResourceInventory))
	switch inventory {
	case "", "{}", "[]", "null", `"null"`:
		return false, nil
	}
	return true, nil
}

// OnCreateConfirmed copies connection info onto the related machine runtime
// instance and clears its Reconciled flag.
func (g *gceMachineLifecycle) OnCreateConfirmed(_ provider.InfraProvider) error {
	latest, err := client.GetGcpGceMachineRuntimeInstanceByID(
		g.r.APIClient,
		g.r.APIServer,
		g.instanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get GCE instance for machine runtime update: %w", err)
	}
	if latest.MachineRuntimeInstanceID == nil {
		return fmt.Errorf("failed to update machine runtime instance: machine runtime instance id is empty")
	}

	// get related machine runtime instance
	machineRuntimeInstance, err := client.GetMachineRuntimeInstanceByID(
		g.r.APIClient,
		g.r.APIServer,
		*latest.MachineRuntimeInstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get machine runtime instance: %w", err)
	}

	// copy SSH connection fields; MRI hostname is ExternalIP
	machineRuntimeInstance.Hostname = latest.ExternalIP
	machineRuntimeInstance.SSHUser = latest.SSHUser
	machineRuntimeInstance.SSHKey = latest.SSHKey
	machineRuntimeInstance.Reconciled = util.Ptr(false)
	if _, err = client.UpdateMachineRuntimeInstance(
		g.r.APIClient,
		g.r.APIServer,
		machineRuntimeInstance,
	); err != nil {
		return fmt.Errorf("failed to update machine runtime instance with connection info: %w", err)
	}
	return nil
}

// SaveCreateOutputs writes hostname, external IP, SSH key, and final state
// onto the GCE machine runtime instance.
func (g *gceMachineLifecycle) SaveCreateOutputs(infra provider.InfraProvider, state *datatypes.JSON) error {
	gceInfra, ok := infra.(*machine.GceMachineInfra)
	if !ok {
		return errors.New("failed to save GCE create outputs: expected *machine.GceMachineInfra")
	}

	hostname, externalIP, sshKey := gceInfra.CreateOutputs()
	updatedInstance := v0.GcpGceMachineRuntimeInstance{
		Common:            v0.Common{ID: &g.instanceID},
		Hostname:          &hostname,
		ExternalIP:        &externalIP,
		SSHKey:            &sshKey,
		ResourceInventory: state,
	}
	if _, err := client.UpdateGcpGceMachineRuntimeInstance(
		g.r.APIClient,
		g.r.APIServer,
		&updatedInstance,
	); err != nil {
		return fmt.Errorf("failed to update GCE instance with create outputs: %w", err)
	}

	// persist the GCE resource inventory onto the abstract machine runtime
	// instance so a downstream consumer can read the payload without
	// crossing into the provider-specific row. skip when no parent link is
	// present, which is the shape a fixture or a partial create can produce
	if g.instance == nil || g.instance.MachineRuntimeInstanceID == nil {
		return nil
	}
	inventory := gceInfra.BuildResourceInventory()
	inventoryBytes, err := json.Marshal(inventory)
	if err != nil {
		return fmt.Errorf("failed to marshal GCE resource inventory: %w", err)
	}
	inventoryJSON := datatypes.JSON(inventoryBytes)
	mriUpdate := v0.MachineRuntimeInstance{
		Common:            v0.Common{ID: g.instance.MachineRuntimeInstanceID},
		ResourceInventory: &inventoryJSON,
	}
	if _, err := client.UpdateMachineRuntimeInstance(
		g.r.APIClient,
		g.r.APIServer,
		&mriUpdate,
	); err != nil {
		return fmt.Errorf("failed to update machine runtime instance with resource inventory: %w", err)
	}
	return nil
}

// OnDeleteConfirmed validates against the compute API that the machine's VM and
// firewall are actually gone after destroy, and reclaims any the destroy left
// behind, so a checkpoint that drifted out of sync with the cloud cannot confirm
// a deletion that abandoned a live resource.
func (g *gceMachineLifecycle) OnDeleteConfirmed(infra provider.InfraProvider) error {
	gceInfra, ok := infra.(*machine.GceMachineInfra)
	if !ok {
		return errors.New("failed to reclaim GCE orphans: expected *machine.GceMachineInfra")
	}
	cloud, err := newComputeOrphanReclaimCloud(gceInfra)
	if err != nil {
		return err
	}
	return reclaimOrphans(cloud)
}

// AckCreation sets CreationAcknowledged and clears CreationFailed.
func (g *gceMachineLifecycle) AckCreation() error {
	ackTimestamp := time.Now().UTC()
	creationFailed := false
	ackUpdate := v0.GcpGceMachineRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			CreationAcknowledged: &ackTimestamp,
			CreationFailed:       &creationFailed,
		},
	}
	_, err := client.UpdateGcpGceMachineRuntimeInstance(g.r.APIClient, g.r.APIServer, &ackUpdate)
	return err
}

// RefreshCreationAck updates CreationAcknowledged to prevent stale detection.
func (g *gceMachineLifecycle) RefreshCreationAck() error {
	refreshTimestamp := time.Now().UTC()
	ackUpdate := v0.GcpGceMachineRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			CreationAcknowledged: &refreshTimestamp,
		},
	}
	_, err := client.UpdateGcpGceMachineRuntimeInstance(g.r.APIClient, g.r.APIServer, &ackUpdate)
	return err
}

// SetCreationFailed marks CreationFailed=true in the API.
func (g *gceMachineLifecycle) SetCreationFailed() error {
	creationFailed := true
	failedUpdate := v0.GcpGceMachineRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			CreationFailed: &creationFailed,
		},
	}
	_, err := client.UpdateGcpGceMachineRuntimeInstance(g.r.APIClient, g.r.APIServer, &failedUpdate)
	return err
}

// SetDeletionFailed marks DeletionFailed=true in the API.
func (g *gceMachineLifecycle) SetDeletionFailed() error {
	deletionFailed := true
	failedUpdate := v0.GcpGceMachineRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			DeletionFailed: &deletionFailed,
		},
	}
	_, err := client.UpdateGcpGceMachineRuntimeInstance(g.r.APIClient, g.r.APIServer, &failedUpdate)
	return err
}

// RecordSuccessfulCreate records a CreateSuccessful event when provisioning
// finishes. ConfirmCreation sets Reconciled first, so a redelivered pass's
// wasReconciled gate skips the generated wrapper emit.
func (g *gceMachineLifecycle) RecordSuccessfulCreate() error {
	return g.r.EventsRecorder.RecordEvent(
		&v0.Event{
			Type:   util.Ptr(event.TypeNormal),
			Reason: util.Ptr(event.ReasonCreateSuccessful),
			Note:   util.Ptr("provisioning complete"),
		},
		g.instance.GetId(),
		g.instance.GetFullyQualifiedType(),
	)
}

// ConfirmCreation sets CreationConfirmed and Reconciled=true.
func (g *gceMachineLifecycle) ConfirmCreation() error {
	reconciled := true
	timestamp := time.Now().UTC()
	confirmedUpdate := v0.GcpGceMachineRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			Reconciled:        &reconciled,
			CreationConfirmed: &timestamp,
		},
	}
	_, err := client.UpdateGcpGceMachineRuntimeInstance(g.r.APIClient, g.r.APIServer, &confirmedUpdate)
	return err
}

// AckDeletion sets DeletionAcknowledged in the API.
func (g *gceMachineLifecycle) AckDeletion() error {
	timestamp := time.Now().UTC()
	ackUpdate := v0.GcpGceMachineRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			DeletionAcknowledged: &timestamp,
		},
	}
	_, err := client.UpdateGcpGceMachineRuntimeInstance(g.r.APIClient, g.r.APIServer, &ackUpdate)
	return err
}

// RefreshDeletionAck updates DeletionAcknowledged to prevent stale detection.
func (g *gceMachineLifecycle) RefreshDeletionAck() error {
	refreshTimestamp := time.Now().UTC()
	ackUpdate := v0.GcpGceMachineRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			DeletionAcknowledged: &refreshTimestamp,
		},
	}
	_, err := client.UpdateGcpGceMachineRuntimeInstance(g.r.APIClient, g.r.APIServer, &ackUpdate)
	return err
}

// ConfirmDeletion sets DeletionConfirmed in the API.
func (g *gceMachineLifecycle) ConfirmDeletion() error {
	timestamp := time.Now().UTC()
	confirmedUpdate := v0.GcpGceMachineRuntimeInstance{
		Common: v0.Common{ID: &g.instanceID},
		Reconciliation: v0.Reconciliation{
			DeletionConfirmed: &timestamp,
		},
	}
	_, err := client.UpdateGcpGceMachineRuntimeInstance(g.r.APIClient, g.r.APIServer, &confirmedUpdate)
	return err
}

// SaveState persists intermediate Pulumi state to the API.
func (g *gceMachineLifecycle) SaveState(state *datatypes.JSON) error {
	stateUpdate := v0.GcpGceMachineRuntimeInstance{
		Common:            v0.Common{ID: &g.instanceID},
		ResourceInventory: state,
	}
	_, err := client.UpdateGcpGceMachineRuntimeInstance(g.r.APIClient, g.r.APIServer, &stateUpdate)
	return err
}

// ClearInventory sets ResourceInventory to "{}" to signal destroy complete.
func (g *gceMachineLifecycle) ClearInventory() error {
	emptyInventory := datatypes.JSON([]byte("{}"))
	clearedUpdate := v0.GcpGceMachineRuntimeInstance{
		Common:            v0.Common{ID: &g.instanceID},
		ResourceInventory: &emptyInventory,
	}
	_, err := client.UpdateGcpGceMachineRuntimeInstance(g.r.APIClient, g.r.APIServer, &clearedUpdate)
	return err
}

// PublishCreateNotification publishes a NATS notification for creation.
func (g *gceMachineLifecycle) PublishCreateNotification() error {
	notifPayload, err := g.instance.NotificationPayload(
		notifications.NotificationOperationCreated,
		false,
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to create notification payload: %w", err)
	}
	if _, err = g.r.JetStreamContext.Publish(
		notif.GcpGceMachineRuntimeInstanceCreateSubject,
		*notifPayload,
	); err != nil {
		return fmt.Errorf("failed to publish create notification: %w", err)
	}
	return nil
}

// PublishDeleteNotification publishes a NATS notification for deletion.
func (g *gceMachineLifecycle) PublishDeleteNotification() error {
	notifPayload, err := g.instance.NotificationPayload(
		notifications.NotificationOperationDeleted,
		false,
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to create notification payload: %w", err)
	}
	if _, err = g.r.JetStreamContext.Publish(
		notif.GcpGceMachineRuntimeInstanceDeleteSubject,
		*notifPayload,
	); err != nil {
		return fmt.Errorf("failed to publish delete notification: %w", err)
	}
	return nil
}

// buildGceMachineInfra constructs a GceMachineInfra from API objects.
func buildGceMachineInfra(
	r *controller.Reconciler,
	instance *v0.GcpGceMachineRuntimeInstance,
	log *logr.Logger,
) (*machine.GceMachineInfra, error) {
	if instance.Name == nil {
		return nil, fmt.Errorf("failed to build gce machine infra: instance name is empty")
	}
	if instance.GcpProviderID == nil {
		return nil, fmt.Errorf("failed to build gce machine infra: gcp provider id is empty")
	}

	// get GCP provider
	gcpProvider, err := client.GetGcpProviderByID(
		r.APIClient,
		r.APIServer,
		*instance.GcpProviderID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve GCP provider by ID: %w", err)
	}
	if gcpProvider.ProjectID == nil {
		return nil, fmt.Errorf("failed to build gce machine infra: gcp project id is empty")
	}

	infraGce := machine.NewGceMachineInfra(*instance.Name)
	infraGce.Logger = log
	infraGce.ProjectID = *gcpProvider.ProjectID

	if instance.GcpGceMachineRuntimeDefinitionID == nil {
		return nil, fmt.Errorf("failed to build gce machine infra: gce machine runtime definition id is empty")
	}
	// get GCE machine runtime definition
	definition, err := client.GetGcpGceMachineRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*instance.GcpGceMachineRuntimeDefinitionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve GCE machine runtime definition by ID: %w", err)
	}
	if definition.MachineType != nil {
		infraGce.MachineType = *definition.MachineType
	}
	// default the boot image when the definition leaves ImageID unset
	if definition.ImageID != nil && *definition.ImageID != "" {
		infraGce.ImageID = *definition.ImageID
	} else {
		infraGce.ImageID = defaultGceImageID
	}

	if instance.Region != nil {
		infraGce.Region = *instance.Region
	}
	if instance.Zone != nil {
		infraGce.Zone = *instance.Zone
	}
	if instance.SSHUser != nil {
		infraGce.SSHUser = *instance.SSHUser
	}

	// load the parent machine runtime instance so NetworkID, SubnetID,
	// IngressRules, NetworkCIDR, SubnetCIDR, and AssignPublicIP are read from
	// the abstract instance; a nil MachineRuntimeInstanceID leaves those
	// fields at their zero values.
	var mri *v0.MachineRuntimeInstance
	if instance.MachineRuntimeInstanceID != nil {
		var err error
		mri, err = client.GetMachineRuntimeInstanceByID(
			r.APIClient,
			r.APIServer,
			*instance.MachineRuntimeInstanceID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve machine runtime instance by ID: %w", err)
		}
	}
	if gcpProvider.ServiceAccountCredentials == nil || *gcpProvider.ServiceAccountCredentials == "" {
		return nil, fmt.Errorf("gcp provider %s has no service account credentials", *gcpProvider.Name)
	}
	// decrypt service account credentials
	decryptedCredentials, err := encryption.Decrypt(r.EncryptionKey, *gcpProvider.ServiceAccountCredentials)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt gcp provider service account credentials: %w", err)
	}
	infraGce.ServiceAccountCredentials = decryptedCredentials

	// translate each portable ingress rule to the provider-side shape; each
	// non-nil string/slice field is copied out of its pointer so a partial
	// rule renders as an empty value on the provider rather than a panic.
	if mri != nil && mri.IngressRules != nil {
		for _, rule := range *mri.IngressRules {
			gceRule := machine.GceIngressRule{}
			if rule.Protocol != nil {
				gceRule.Protocol = *rule.Protocol
			}
			if rule.Ports != nil {
				gceRule.Ports = append([]string(nil), *rule.Ports...)
			}
			if rule.SourceRanges != nil {
				gceRule.SourceRanges = append([]string(nil), *rule.SourceRanges...)
			}
			if rule.Description != nil {
				gceRule.Description = *rule.Description
			}
			infraGce.IngressRules = append(infraGce.IngressRules, gceRule)
		}
	}

	if mri != nil {
		if mri.NetworkID != nil {
			infraGce.NetworkID = *mri.NetworkID
		}
		if mri.SubnetID != nil {
			infraGce.SubnetID = *mri.SubnetID
		}
		if mri.NetworkCIDR != nil {
			infraGce.NetworkCIDR = *mri.NetworkCIDR
		}
		if mri.SubnetCIDR != nil {
			infraGce.SubnetCIDR = *mri.SubnetCIDR
		}
		if mri.AssignPublicIP != nil {
			infraGce.AssignPublicIP = *mri.AssignPublicIP
		}
	}

	// require service account credentials so create does not fall through to ambient ADC
	if gcpProvider.ServiceAccountCredentials == nil || *gcpProvider.ServiceAccountCredentials == "" {
		if gcpProvider.ID == nil {
			return nil, errors.New("gcp provider has no service account credentials")
		}
		return nil, fmt.Errorf("gcp provider %d has no service account credentials", *gcpProvider.ID)
	}
	decryptedCredentials, decryptErr := encryption.Decrypt(r.EncryptionKey, *gcpProvider.ServiceAccountCredentials)
	if decryptErr != nil {
		return nil, fmt.Errorf("failed to decrypt gcp provider service account credentials: %w", decryptErr)
	}
	infraGce.ServiceAccountCredentials = decryptedCredentials

	// rehydrate the persisted SSH key onto the rebuilt provider so a re-deploy
	// reuses it instead of minting a fresh pair and rotating the instance's
	// authorized key away from the key this control plane holds. The stored key
	// is encrypted at rest, so decrypt it first; it is unset on a clean first
	// create, in which case the provider generates a new pair.
	if instance.SSHKey != nil && *instance.SSHKey != "" {
		// decrypt and seed persisted SSH key
		decryptedKey, err := encryption.Decrypt(r.EncryptionKey, *instance.SSHKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt persisted GCE SSH key: %w", err)
		}
		if err := infraGce.SeedSSHKeyPair(decryptedKey); err != nil {
			return nil, fmt.Errorf("failed to seed persisted GCE SSH key: %w", err)
		}
	}

	// persist a newly generated key onto the instance before Pulumi up
	infraGce.PersistSSHKey = func(pem string) error {
		if instance.ID == nil {
			return fmt.Errorf("failed to persist GCE SSH key: instance id is empty")
		}
		updated := v0.GcpGceMachineRuntimeInstance{
			Common: v0.Common{ID: instance.ID},
			SSHKey: &pem,
		}
		if _, err := client.UpdateGcpGceMachineRuntimeInstance(
			r.APIClient,
			r.APIServer,
			&updated,
		); err != nil {
			return fmt.Errorf("failed to persist GCE SSH key: %w", err)
		}
		return nil
	}

	return infraGce, nil
}
