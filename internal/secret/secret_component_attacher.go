package secret

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// secretComponentAttacher is the caller that records the shared secrets component.
type secretComponentAttacher struct {
	config *SecretInstanceConfig
}

// GetReconciler returns the reconciler that reaches the API.
func (a *secretComponentAttacher) GetReconciler() *controller.Reconciler {
	return a.config.r
}

// Install creates the secrets component for this runtime.
func (a *secretComponentAttacher) Install(
	runtime *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) (uint, error) {
	// build the secrets component manifest for the provider
	var externalSecretsYaml string
	var err error
	switch *a.config.kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS, v0.KubernetesRuntimeInfraProviderKind:
		externalSecretsYaml, err = a.config.getExternalSecretsSupportServiceYaml()
	default:
		break
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get external secrets yaml: %w", err)
	}

	// reuse the workload definition a deleted component left behind
	var createdWorkloadDef *v0.KubernetesWorkloadDefinition
	workloadDefName := fmt.Sprintf("%s-%s", "external-secrets", *runtime.Name)
	existingDefinition, err := client.GetKubernetesWorkloadDefinitionByName(
		a.config.r.APIClient,
		a.config.r.APIServer,
		workloadDefName,
	)
	if err != nil && !errors.Is(err, client_lib.ErrObjectNotFound) {
		return 0, fmt.Errorf("failed to get secrets component kubernetes workload definition: %w", err)
	}
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		workloadDefinition := v0.KubernetesWorkloadDefinition{
			Definition:   v0.Definition{Name: &workloadDefName},
			YAMLDocument: &externalSecretsYaml,
		}
		createdWorkloadDef, err = client.CreateKubernetesWorkloadDefinition(
			a.config.r.APIClient,
			a.config.r.APIServer,
			&workloadDefinition,
		)
		if err != nil {
			return 0, fmt.Errorf(
				"failed to create secrets component kubernetes workload definition: %w",
				err,
			)
		}
	} else {
		createdWorkloadDef = existingDefinition
	}

	// create the secrets component workload instance
	workloadInstance := v0.KubernetesWorkloadInstance{
		Instance:                       v0.Instance{Name: &workloadDefName},
		KubernetesRuntimeInstanceID:    runtime.ID,
		KubernetesWorkloadDefinitionID: createdWorkloadDef.ID,
	}
	created, err := client.CreateKubernetesWorkloadInstance(
		a.config.r.APIClient,
		a.config.r.APIServer,
		&workloadInstance,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"failed to create secrets component kubernetes workload instance: %w",
			err,
		)
	}
	log.V(1).Info("secrets component deployed", "workloadInstanceID", *created.ID)
	return *created.ID, nil
}

// IsComponentReconciled reports whether the workload has reconciled.
// A missing row returns reconciled false and missing true.
func (a *secretComponentAttacher) IsComponentReconciled(id uint) (bool, bool, error) {
	// read the workload the slot names
	workload, err := client.GetKubernetesWorkloadInstanceByID(
		a.config.r.APIClient,
		a.config.r.APIServer,
		id,
	)
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		return false, true, nil
	}
	if err != nil {
		return false, false, err
	}
	if workload.DeletionScheduled != nil {
		return false, false, fmt.Errorf("secrets component workload %d is being deleted", id)
	}
	if workload.Reconciled != nil && *workload.Reconciled {
		return true, false, nil
	}
	return false, false, nil
}

// SetCallerKubernetesWorkloadInstanceID records nothing because the secret stores no component id.
func (a *secretComponentAttacher) SetCallerKubernetesWorkloadInstanceID(id *uint) error {
	return nil
}

// ClaimRuntimeKubernetesWorkloadInstanceID writes the runtime slot.
func (a *secretComponentAttacher) ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) error {
	// load the runtime and set the slot
	runtime, err := client.GetKubernetesRuntimeInstanceByID(
		a.config.r.APIClient,
		a.config.r.APIServer,
		runtimeID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance %d: %w", runtimeID, err)
	}
	runtime.SecretsComponentInstanceID = util.Ptr(workloadID)
	_, err = client.UpdateKubernetesRuntimeInstance(
		a.config.r.APIClient,
		a.config.r.APIServer,
		runtime,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to claim secrets component slot on runtime %d: %w",
			runtimeID,
			err,
		)
	}
	return nil
}

// ClearRuntimeKubernetesWorkloadInstanceID clears the runtime slot when no caller requires the workload.
func (a *secretComponentAttacher) ClearRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) (bool, error) {
	// load the runtime
	runtime, err := client.GetKubernetesRuntimeInstanceByID(
		a.config.r.APIClient,
		a.config.r.APIServer,
		runtimeID,
	)
	if err != nil {
		return false, fmt.Errorf(
			"failed to get kubernetes runtime instance %d: %w",
			runtimeID,
			err,
		)
	}
	if runtime.SecretsComponentInstanceID == nil ||
		*runtime.SecretsComponentInstanceID != workloadID {
		return false, nil
	}

	// replace the slot with null
	body, err := secretsComponentSlotNull(runtime)
	if err != nil {
		return false, err
	}
	_, err = client_lib.GetResponse(
		a.config.r.APIClient,
		fmt.Sprintf("%s%s/%d", a.config.r.APIServer, v0.PathKubernetesRuntimeInstances, runtimeID),
		http.MethodPut,
		bytes.NewBuffer(body),
		map[string]string{},
		http.StatusOK,
	)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, client_lib.ErrBadRequest) &&
		strings.Contains(err.Error(), v0.ErrMsgSupportServiceSlotHeld) {
		return false, nil
	}
	return false, fmt.Errorf(
		"failed to clear secrets component slot on runtime %d: %w",
		runtimeID,
		err,
	)
}

// DeleteComponentKubernetesWorkloadInstance removes the workload instance.
func (a *secretComponentAttacher) DeleteComponentKubernetesWorkloadInstance(id uint) error {
	// treat a missing workload as already removed
	_, err := client.DeleteKubernetesWorkloadInstance(
		a.config.r.APIClient,
		a.config.r.APIServer,
		id,
	)
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil
	}
	return err
}

// secretsComponentSlotNull encodes the runtime with the secrets slot set to null.
func secretsComponentSlotNull(runtime *v0.KubernetesRuntimeInstance) ([]byte, error) {
	// encode the runtime and set the slot to null
	encoded, err := json.Marshal(runtime)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal kubernetes runtime instance: %w", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kubernetes runtime instance: %w", err)
	}
	delete(fields, "ID")
	delete(fields, "CreatedAt")
	delete(fields, "UpdatedAt")
	fields["SecretsComponentInstanceID"] = nil
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal kubernetes runtime instance: %w", err)
	}
	return body, nil
}
