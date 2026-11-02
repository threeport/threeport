// Package supportservices installs and removes the one workload a runtime shares across callers.
package supportservices

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	api "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// SupportServiceAttacher is the object that uses one shared workload.
type SupportServiceAttacher interface {

	// IsComponentReconciled reports whether that kubernetes workload instance has reconciled.
	// A missing row returns reconciled false and missing true.
	IsComponentReconciled(id uint) (reconciled bool, missing bool, err error)

	// SetCallerKubernetesWorkloadInstanceID stores the component kubernetes workload instance id on the caller.
	SetCallerKubernetesWorkloadInstanceID(id *uint) error
	
	// ClaimRuntimeKubernetesWorkloadInstanceID writes the runtime component column when the compare accepts it.
	ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) error

	// ClearRuntimeKubernetesWorkloadInstanceID nulls the runtime component column when this id is the stored one.
	// The cleared result is true when this caller nulled the column.
	ClearRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) (cleared bool, err error)

	// DeleteComponentKubernetesWorkloadInstance removes that component kubernetes workload instance.
	DeleteComponentKubernetesWorkloadInstance(id uint) error

	// GetReconciler returns the reconciler this caller uses for API calls.
	GetReconciler() *controller.Reconciler
}

// SupportServiceInstaller is an attacher that creates the shared workload.
type SupportServiceInstaller interface {
	SupportServiceAttacher
	// Install creates the shared workload and returns its id.
	Install(runtime *api.KubernetesRuntimeInstance, log *logr.Logger) (uint, error)
}

// EnsureIngressComponent installs or adopts the ingress component for this runtime.
// The caller records that workload on the object that uses it.
func EnsureIngressComponent(
	runtime *api.KubernetesRuntimeInstance,
	caller SupportServiceInstaller,
	log *logr.Logger,
) error {
	// require the runtime the slot belongs to
	if runtime == nil || runtime.ID == nil {
		return fmt.Errorf("ingress component runtime is missing")
	}

	// install or adopt the shared workload
	return ensure(
		runtime,
		&runtime.IngressComponentInstanceID,
		"ingress component",
		caller,
		log,
	)
}

// ReleaseIngressComponent removes the ingress component when no caller still requires it.
func ReleaseIngressComponent(
	runtime *api.KubernetesRuntimeInstance,
	caller SupportServiceAttacher,
) error {
	// require the runtime the slot belongs to
	if runtime == nil || runtime.ID == nil {
		return fmt.Errorf("ingress component runtime is missing")
	}

	// clear the slot, then delete the workload
	return release(
		*runtime.ID,
		&runtime.IngressComponentInstanceID,
		"ingress component",
		caller,
	)
}

// EnsureSecretsComponent installs or adopts the secrets workload for this runtime.
// The caller records that workload on the object that uses it.
func EnsureSecretsComponent(
	runtime *api.KubernetesRuntimeInstance,
	caller SupportServiceInstaller,
	log *logr.Logger,
) error {
	// require the runtime the slot belongs to
	if runtime == nil || runtime.ID == nil {
		return fmt.Errorf("secrets component runtime is missing")
	}

	// install or adopt the shared workload
	return ensure(
		runtime,
		&runtime.SecretsComponentInstanceID,
		"secrets component",
		caller,
		log,
	)
}

// ReleaseSecretsComponent removes the secrets workload when no caller still requires it.
func ReleaseSecretsComponent(
	runtime *api.KubernetesRuntimeInstance,
	caller SupportServiceAttacher,
) error {
	// require the runtime the slot belongs to
	if runtime == nil || runtime.ID == nil {
		return fmt.Errorf("secrets component runtime is missing")
	}

	// clear the slot, then delete the workload
	return release(
		*runtime.ID,
		&runtime.SecretsComponentInstanceID,
		"secrets component",
		caller,
	)
}

// EnsureDatabaseComponent installs or adopts the database workload for this runtime.
// The caller records that workload on the object that uses it.
func EnsureDatabaseComponent(
	runtime *api.KubernetesRuntimeInstance,
	caller SupportServiceInstaller,
	log *logr.Logger,
) error {
	// require the runtime the slot belongs to
	if runtime == nil || runtime.ID == nil {
		return fmt.Errorf("database component runtime is missing")
	}

	// install or adopt the shared workload
	return ensure(
		runtime,
		&runtime.DatabaseComponentInstanceID,
		"database component",
		caller,
		log,
	)
}

// ReleaseDatabaseComponent removes the database workload when no caller still requires it.
func ReleaseDatabaseComponent(
	runtime *api.KubernetesRuntimeInstance,
	caller SupportServiceAttacher,
) error {
	// require the runtime the slot belongs to
	if runtime == nil || runtime.ID == nil {
		return fmt.Errorf("database component runtime is missing")
	}

	// clear the slot, then delete the workload
	return release(
		*runtime.ID,
		&runtime.DatabaseComponentInstanceID,
		"database component",
		caller,
	)
}

// EnsureCertificatesComponent installs or adopts the certificates component for this runtime.
// The caller records that workload on the object that uses it.
func EnsureCertificatesComponent(
	runtime *api.KubernetesRuntimeInstance,
	caller SupportServiceInstaller,
	log *logr.Logger,
) error {
	// require the runtime the slot belongs to
	if runtime == nil || runtime.ID == nil {
		return fmt.Errorf("certificates component runtime is missing")
	}

	// install or adopt the shared workload
	return ensure(
		runtime,
		&runtime.CertificatesComponentInstanceID,
		"certificates component",
		caller,
		log,
	)
}

// ReleaseCertificatesComponent removes the certificates component when no caller still requires it.
func ReleaseCertificatesComponent(
	runtime *api.KubernetesRuntimeInstance,
	caller SupportServiceAttacher,
) error {
	// require the runtime the slot belongs to
	if runtime == nil || runtime.ID == nil {
		return fmt.Errorf("certificates component runtime is missing")
	}

	// clear the slot, then delete the workload
	return release(
		*runtime.ID,
		&runtime.CertificatesComponentInstanceID,
		"certificates component",
		caller,
	)
}

// ensure installs the named workload when slot is empty.
// It stores the workload id in slot and returns an error until that workload is reconciled.
func ensure(
	runtime *api.KubernetesRuntimeInstance,
	slot **uint,
	label string,
	caller SupportServiceInstaller,
	log *logr.Logger,
) error {
	// keep a reconciled workload and wait while its row still exists
	if *slot != nil {
		reconciled, missing, err := caller.IsComponentReconciled(**slot)
		if err != nil {
			return fmt.Errorf("failed to read %s workload %d: %w", label, **slot, err)
		}
		if reconciled {
			if err := caller.SetCallerKubernetesWorkloadInstanceID(*slot); err != nil {
				return fmt.Errorf(
					"failed to record %s workload %d: %w",
					label,
					**slot,
					err,
				)
			}
			return nil
		}
		if !missing {
			return fmt.Errorf("%s workload %d is not reconciled", label, **slot)
		}
	}

	// install the workload and claim the slot
	workloadID, err := caller.Install(runtime, log)
	if err != nil {
		return err
	}
	if err := caller.ClaimRuntimeKubernetesWorkloadInstanceID(*runtime.ID, workloadID); err != nil {
		if deleteErr := caller.DeleteComponentKubernetesWorkloadInstance(workloadID); deleteErr != nil {
			return fmt.Errorf(
				"failed to delete unclaimed %s workload %d: %w",
				label,
				workloadID,
				deleteErr,
			)
		}
		return err
	}
	*slot = util.Ptr(workloadID)
	if err := caller.SetCallerKubernetesWorkloadInstanceID(*slot); err != nil {
		return fmt.Errorf("failed to record %s workload %d: %w", label, workloadID, err)
	}
	return fmt.Errorf("%s workload %d is not reconciled", label, workloadID)
}

// release removes the named workload when no caller still requires it.
// A failed delete restores the slot.
func release(runtimeID uint, slot **uint, label string, caller SupportServiceAttacher) error {
	// clear the slot when it names a workload
	if *slot == nil {
		return nil
	}
	workloadID := **slot
	cleared, err := caller.ClearRuntimeKubernetesWorkloadInstanceID(runtimeID, workloadID)
	if err != nil {
		return fmt.Errorf("failed to clear %s slot: %w", label, err)
	}
	if !cleared {
		return nil
	}

	// delete the workload, and put the slot back when the delete fails
	if err := caller.DeleteComponentKubernetesWorkloadInstance(workloadID); err != nil {
		if claimErr := caller.ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID, workloadID); claimErr != nil {
			return fmt.Errorf(
				"failed to delete %s workload %d: %w",
				label,
				workloadID,
				errors.Join(err, claimErr),
			)
		}
		return fmt.Errorf(
			"failed to delete %s workload %d: %w",
			label,
			workloadID,
			err,
		)
	}
	*slot = nil
	return nil
}
