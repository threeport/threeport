package secret

import (
	"fmt"

	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"github.com/threeport/threeport/pkg/supportservices"
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
		externalSecretsYaml, err = supportservices.ExternalSecretsYaml()
	default:
		break
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get external secrets yaml: %w", err)
	}

	return supportservices.CreateComponentWorkload(
		a.config.r,
		runtime,
		fmt.Sprintf("external-secrets-%s", *runtime.Name),
		externalSecretsYaml,
		log,
	)
}

// IsComponentReconciled reports whether the workload has reconciled.
// A missing row returns reconciled false and missing true.
func (a *secretComponentAttacher) IsComponentReconciled(id uint) (bool, bool, error) {
	return supportservices.ComponentReconciled(a.config.r, id)
}

// SetCallerKubernetesWorkloadInstanceID records nothing because the secret stores no component id.
func (a *secretComponentAttacher) SetCallerKubernetesWorkloadInstanceID(id *uint) error {
	return nil
}

// ClaimRuntimeKubernetesWorkloadInstanceID writes the runtime slot.
func (a *secretComponentAttacher) ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) error {
	return supportservices.ClaimComponentWorkload(
		a.config.r,
		runtimeID,
		workloadID,
		supportservices.SecretsColumn,
	)
}

// ClearRuntimeKubernetesWorkloadInstanceID clears the runtime slot when no caller requires the workload.
func (a *secretComponentAttacher) ClearRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) (bool, error) {
	return supportservices.ClearComponentWorkload(
		a.config.r,
		runtimeID,
		workloadID,
		supportservices.SecretsColumn,
	)
}

// DeleteComponentKubernetesWorkloadInstance removes the workload instance.
func (a *secretComponentAttacher) DeleteComponentKubernetesWorkloadInstance(id uint) error {
	return supportservices.DeleteComponentWorkload(a.config.r, id)
}
