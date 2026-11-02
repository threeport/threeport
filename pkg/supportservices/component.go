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

// EnsureIngressComponent installs or adopts the ingress component for this runtime.
// The caller records that workload on the object that uses it.
func EnsureIngressComponent(
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	caller SupportServiceAttacher,
	log *logr.Logger,
) error {
	// require the runtime the slot belongs to
	if runtime == nil || runtime.ID == nil || runtime.Name == nil {
		return fmt.Errorf("ingress component runtime is missing")
	}

	// install or adopt the shared workload
	name := fmt.Sprintf("%s-%s", IngressComponentName, *runtime.Name)
	return ensureComponent(
		r,
		runtime,
		&runtime.IngressComponentInstanceID,
		"ingress component",
		name,
		IngressComponentYaml,
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
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	provider *string,
	caller SupportServiceAttacher,
	log *logr.Logger,
) error {
	// require the runtime the slot belongs to
	if runtime == nil || runtime.ID == nil || runtime.Name == nil {
		return fmt.Errorf("secrets component runtime is missing")
	}

	// skip a provider that does not run this workload
	if !secretsProviderCreates(provider) {
		return nil
	}

	// install or adopt the shared workload
	name := fmt.Sprintf("external-secrets-%s", *runtime.Name)
	return ensureComponent(
		r,
		runtime,
		&runtime.SecretsComponentInstanceID,
		"secrets component",
		name,
		ExternalSecretsYaml,
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
	caller SupportServiceAttacher,
	log *logr.Logger,
) error {
	// require the runtime the slot belongs to
	if runtime == nil || runtime.ID == nil {
		return fmt.Errorf("database component runtime is missing")
	}

	// the database component has no manifest to install
	return fmt.Errorf("database component manifest is missing")
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
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	caller SupportServiceAttacher,
	log *logr.Logger,
) error {
	// require the runtime the slot belongs to
	if runtime == nil || runtime.ID == nil || runtime.Name == nil {
		return fmt.Errorf("certificates component runtime is missing")
	}

	// install or adopt the shared workload
	name := fmt.Sprintf("%s-%s", CertificatesComponentName, *runtime.Name)
	return ensureComponent(
		r,
		runtime,
		&runtime.CertificatesComponentInstanceID,
		"certificates component",
		name,
		CertificatesComponentYaml,
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
	caller SupportServiceAttacher,
	log *logr.Logger,
	manifest func() (string, error),
	create func(manifest string) (uint, error),
	sync func(workloadID uint, manifest string) error,
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
			document, err := componentDocument(caller, manifest)
			if err != nil {
				return fmt.Errorf("failed to render %s workload %d: %w", label, **slot, err)
			}
			if err := sync(**slot, document); err != nil {
				return fmt.Errorf("failed to sync %s workload %d: %w", label, **slot, err)
			}
			return nil
		}
		if !missing {
			return fmt.Errorf("%s workload %d is not reconciled", label, **slot)
		}
	}

	// install the component manifest and claim the slot
	document, err := manifest()
	if err != nil {
		return fmt.Errorf("failed to render %s workload: %w", label, err)
	}
	workloadID, err := create(document)
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

// ensureComponent creates the component workload and syncs the caller's document.
func ensureComponent(
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	slot **uint,
	label string,
	name string,
	manifest func() (string, error),
	caller SupportServiceAttacher,
	log *logr.Logger,
) error {
	// wrap the component document on create and sync the caller's copy
	return ensure(
		runtime,
		slot,
		label,
		caller,
		log,
		manifest,
		func(document string) (uint, error) {
			return CreateComponentWorkload(
				r,
				runtime,
				name,
				fmt.Sprintf("---\n%s\n", document),
				log,
			)
		},
		func(workloadID uint, document string) error {
			return SyncWorkloadObject(r, workloadID, document)
		},
	)
}

// componentDocument returns the component manifest, or the caller's edited copy.
func componentDocument(caller SupportServiceAttacher, manifest func() (string, error)) (string, error) {
	// let the caller change the document when it implements that
	editor, ok := caller.(ManifestEditor)
	if !ok {
		return manifest()
	}
	return editor.EditManifest(manifest)
}

// secretsProviderCreates reports whether this provider runs the secrets workload.
func secretsProviderCreates(provider *string) bool {
	// create the workload for eks and kind
	return provider != nil &&
		(*provider == api.KubernetesRuntimeInfraProviderEKS ||
			*provider == api.KubernetesRuntimeInfraProviderKind)
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
