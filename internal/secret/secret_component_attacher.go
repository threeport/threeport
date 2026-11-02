package secret

import (
	"github.com/threeport/threeport/pkg/supportservices"
)

// secretComponentAttacher is the caller that records the shared secrets component.
type secretComponentAttacher struct {
	config *SecretInstanceConfig
}

// Install creates the secrets component for this runtime.
// IsComponentReconciled reports whether the workload has reconciled.
// A missing row returns reconciled false and missing true.
func (a *secretComponentAttacher) IsComponentReconciled(id uint) (bool, bool, error) {
	return supportservices.IsComponentReconciled(a.config.r, id)
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
