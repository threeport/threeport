package supportservices

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	client "github.com/threeport/threeport/pkg/client/v0"

	api "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// EnsureIngressComponent installs or adopts the shared ingress workload and records it on caller.
// A caller that edits the manifest syncs that document. The workload name is ingress-<runtime>.
func EnsureIngressComponent[T componentCaller](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	caller T,
	field **uint,
	update objectUpdate[T],
	log *logr.Logger,
) error {
	// require the runtime before reading its ingress slot
	if err := requireRuntime(runtime, "ingress component", true); err != nil {
		return err
	}

	// record the workload id on the caller's ingress field
	record := recordComponentID(r, caller, field, update)

	// install the ingress workload and sync the caller's document after reconcile
	return ensure(
		r,
		runtime,
		&runtime.IngressComponentInstanceID,
		"ingress component",
		fmt.Sprintf("%s-%s", IngressComponentName, *runtime.Name),
		ingressColumn,
		record,
		callerManifest(caller),
		ingressComponentYaml,
		log,
	)
}

// EnsureCertificatesComponent installs or adopts the shared certificates workload and records it on caller.
// The workload name is certificates-<runtime>, and other providers are left unchanged.
func EnsureCertificatesComponent[T componentCaller](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	caller T,
	field **uint,
	update objectUpdate[T],
	log *logr.Logger,
) error {
	// require the runtime before reading its certificates slot
	if err := requireRuntime(runtime, "certificates component", true); err != nil {
		return err
	}

	// install certificates on eks and kind
	provider, err := client.GetInfraProviderByKubernetesRuntimeInstanceID(
		r.APIClient,
		r.APIServer,
		runtime.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to get infra provider: %w", err)
	}
	if !secretsProviderCreates(provider) {
		return nil
	}

	// record the workload id on the caller's certificates field
	record := recordComponentID(r, caller, field, update)

	// install the certificates workload and sync it after reconcile
	return ensure(
		r,
		runtime,
		&runtime.CertificatesComponentInstanceID,
		"certificates component",
		fmt.Sprintf("%s-%s", CertificatesComponentName, *runtime.Name),
		certificatesColumn,
		record,
		nil,
		certificatesComponentYaml,
		log,
	)
}

// EnsureSecretsComponent installs or adopts the shared secrets workload and records it on caller.
// The workload name is external-secrets-<runtime>, and other providers are left unchanged.
func EnsureSecretsComponent[T componentCaller](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	caller T,
	field **uint,
	update objectUpdate[T],
	provider *string,
	log *logr.Logger,
) error {
	// require the runtime before reading its secrets slot
	if err := requireRuntime(runtime, "secrets component", true); err != nil {
		return err
	}

	// create the workload for eks and kind
	if !secretsProviderCreates(provider) {
		return nil
	}

	// install the external-secrets workload and sync it after reconcile
	return ensure(
		r,
		runtime,
		&runtime.SecretsComponentInstanceID,
		"secrets component",
		fmt.Sprintf("external-secrets-%s", *runtime.Name),
		secretsColumn,
		recordComponentID(r, caller, field, update),
		nil,
		externalSecretsYaml,
		log,
	)
}

// EnsureDatabaseComponent reports that the database component has no manifest.
func EnsureDatabaseComponent[T componentCaller](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	caller T,
	field **uint,
	update objectUpdate[T],
	log *logr.Logger,
) error {
	// require a runtime id before reporting the missing manifest
	if err := requireRuntime(runtime, "database component", false); err != nil {
		return err
	}

	return fmt.Errorf("database component manifest is missing")
}

// ReleaseIngressComponent removes the caller's ingress claim and deletes the workload when the slot is clear.
func ReleaseIngressComponent[T componentCaller](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	caller T,
) error {
	// require the runtime before reading its ingress slot
	if err := requireRuntime(runtime, "ingress component", false); err != nil {
		return err
	}

	return releaseComponent(r, runtime, &runtime.IngressComponentInstanceID, "ingress component", ingressColumn, caller)
}

// ReleaseCertificatesComponent removes the caller's certificates claim and deletes the workload when the slot is clear.
func ReleaseCertificatesComponent[T componentCaller](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	caller T,
) error {
	// require the runtime before reading its certificates slot
	if err := requireRuntime(runtime, "certificates component", false); err != nil {
		return err
	}

	return releaseComponent(r, runtime, &runtime.CertificatesComponentInstanceID, "certificates component", certificatesColumn, caller)
}

// ReleaseSecretsComponent removes the caller's secrets claim and deletes the workload when the slot is clear.
func ReleaseSecretsComponent[T componentCaller](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	caller T,
) error {
	// require the runtime before reading its secrets slot
	if err := requireRuntime(runtime, "secrets component", false); err != nil {
		return err
	}

	return releaseComponent(r, runtime, &runtime.SecretsComponentInstanceID, "secrets component", secretsColumn, caller)
}

// ReleaseDatabaseComponent removes the caller's database claim and deletes the workload when the slot is clear.
func ReleaseDatabaseComponent[T componentCaller](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	caller T,
) error {
	// require the runtime before reading its database slot
	if err := requireRuntime(runtime, "database component", false); err != nil {
		return err
	}

	return releaseComponent(r, runtime, &runtime.DatabaseComponentInstanceID, "database component", databaseColumn, caller)
}

// ensure installs the component when the slot is empty and syncs it after the workload reconciles.
func ensure(
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	slot **uint,
	label string,
	workloadName string,
	column string,
	record func(id *uint) error,
	editor manifestEditor,
	manifest func() (string, error),
	log *logr.Logger,
) error {
	// require the runtime id and name before using the slot
	if runtime == nil || runtime.ID == nil || runtime.Name == nil {
		return fmt.Errorf("%s runtime is missing", label)
	}

	// sync the caller's document once the stored workload has reconciled
	if *slot != nil {
		reconciled, missing, err := isComponentReconciled(r, **slot)
		if err != nil {
			return fmt.Errorf("failed to read %s workload %d: %w", label, **slot, err)
		}
		if reconciled {
			if record != nil {
				if err := record(*slot); err != nil {
					return fmt.Errorf("failed to record %s workload %d: %w", label, **slot, err)
				}
			}
			document, err := componentDocument(editor, manifest)
			if err != nil {
				return err
			}
			return syncWorkloadObject(r, **slot, document)
		}
		if !missing {
			return fmt.Errorf("%s workload %d not reconciled", label, **slot)
		}
	}

	// create the workload from the component manifest
	document, err := manifest()
	if err != nil {
		return err
	}
	workloadID, err := createComponentWorkload(
		r,
		runtime,
		workloadName,
		fmt.Sprintf("---\n%s\n", document),
		log,
	)
	if err != nil {
		return err
	}

	// claim the runtime slot and record the workload on the caller
	if err := claimComponentWorkload(r, *runtime.ID, workloadID, column); err != nil {
		if deleteErr := deleteComponentWorkload(r, workloadID); deleteErr != nil {
			return deleteErr
		}
		return err
	}
	*slot = util.Ptr(workloadID)
	if record != nil {
		if err := record(*slot); err != nil {
			return fmt.Errorf("failed to record %s workload %d: %w", label, workloadID, err)
		}
	}

	return fmt.Errorf("%s workload %d not reconciled", label, workloadID)
}

// recordComponentID stores the workload id on the caller and saves the object.
func recordComponentID[T any](
	r *controller.Reconciler,
	caller T,
	field **uint,
	update objectUpdate[T],
) func(id *uint) error {
	return func(id *uint) error {
		// skip a field that is absent or already holds this workload
		if field == nil {
			return nil
		}
		if *field != nil && id != nil && **field == *id {
			return nil
		}

		// store the id and save the object
		*field = id
		if update == nil {
			return nil
		}
		if _, err := update(r.APIClient, r.APIServer, caller); err != nil {
			return fmt.Errorf("failed to record component workload: %w", err)
		}
		return nil
	}
}

// callerManifest returns the caller's document editor when the caller has one.
func callerManifest(caller any) manifestEditor {
	editor, _ := caller.(manifestEditor)
	return editor
}

// componentDocument returns the editor's document, or the component manifest when there is no editor.
func componentDocument(editor manifestEditor, manifest func() (string, error)) (string, error) {
	// return the component manifest when the caller adds nothing
	if editor == nil {
		return manifest()
	}

	return editor.EditManifest(manifest)
}

// secretsProviderCreates reports whether the provider gets an external-secrets workload.
func secretsProviderCreates(provider *string) bool {
	// create the workload for eks and kind
	return provider != nil &&
		(*provider == api.KubernetesRuntimeInfraProviderEKS ||
			*provider == api.KubernetesRuntimeInfraProviderKind)
}

// releaseComponent drops the caller's requires row and deletes the workload when the slot is clear.
func releaseComponent[T componentCaller](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	slot **uint,
	label string,
	column string,
	caller T,
) error {
	// require the runtime before reading the slot
	if runtime == nil || runtime.ID == nil {
		return fmt.Errorf("%s runtime is missing", label)
	}

	return release(
		r,
		*runtime.ID,
		slot,
		label,
		column,
		caller.GetFullyQualifiedType(),
		caller.ObjectID(),
	)
}

// clearCallerClaim drops this caller's requires row and clears the runtime column.
func clearCallerClaim(
	r *controller.Reconciler,
	runtimeID uint,
	workloadID uint,
	column string,
	attachedType string,
	attachedID *uint,
) (bool, error) {
	// keep the workload when this column holds a different id
	held, err := componentColumnHeld(r, runtimeID, workloadID, column)
	if err != nil || !held {
		return held, err
	}

	// drop this caller's requires row before clearing the column
	if attachedID != nil {
		if err := dropComponentClaim(r, attachedType, *attachedID, workloadID); err != nil {
			return false, err
		}
	}

	return nullComponentColumn(r, runtimeID, column)
}

// release deletes the workload after the caller's claim is cleared.
func release(
	r *controller.Reconciler,
	runtimeID uint,
	slot **uint,
	label string,
	column string,
	attachedType string,
	attachedID *uint,
) error {
	// nothing is installed
	if *slot == nil {
		return nil
	}
	workloadID := **slot

	// delete the workload when this caller held the slot
	cleared, err := clearCallerClaim(
		r,
		runtimeID,
		workloadID,
		column,
		attachedType,
		attachedID,
	)
	if err != nil {
		return fmt.Errorf("failed to clear %s slot: %w", label, err)
	}
	if !cleared {
		return nil
	}

	// delete the workload, and put the claim back when the delete fails
	if err := deleteComponentWorkload(r, workloadID); err != nil {
		if claimErr := claimComponentWorkload(r, runtimeID, workloadID, column); claimErr != nil {
			return fmt.Errorf(
				"failed to delete %s workload %d: %w",
				label,
				workloadID,
				errors.Join(err, claimErr),
			)
		}
		return fmt.Errorf("failed to delete %s workload %d: %w", label, workloadID, err)
	}
	*slot = nil
	return nil
}

// requireRuntime reports a missing runtime id, and a missing name when the workload name uses it.
func requireRuntime(runtime *api.KubernetesRuntimeInstance, label string, nameRequired bool) error {
	// reject a runtime that cannot name the workload
	if runtime == nil || runtime.ID == nil || (nameRequired && runtime.Name == nil) {
		return fmt.Errorf("%s runtime is missing", label)
	}
	return nil
}
