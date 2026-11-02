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

// EnsureIngressComponent attaches an API instance to the ingress component of the support services operator.
// The shared workload is named ingress-<runtime>. An instance that edits the manifest syncs that edit.
func EnsureIngressComponent[T componentAttacher](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	attacher T,
	field **uint,
	update objectUpdate[T],
	log *logr.Logger,
) error {
	// require the runtime before reading its ingress workload id
	if err := requireRuntime(runtime, "ingress component", true); err != nil {
		return err
	}

	// store the ingress workload id on the attached API instance
	record := recordComponentID(r, attacher, field, update)

	// install the ingress component and sync its manifest after reconcile
	return ensure(
		r,
		runtime,
		&runtime.IngressComponentInstanceID,
		"ingress component",
		fmt.Sprintf("%s-%s", IngressComponentName, *runtime.Name),
		ingressColumn,
		record,
		attacherManifest(attacher),
		ingressComponentYaml,
		log,
	)
}

// EnsureCertificatesComponent attaches an API instance to the certificates component of the support services operator.
// The shared workload is named certificates-<runtime>. Providers other than eks and kind are left unchanged.
func EnsureCertificatesComponent[T componentAttacher](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	attacher T,
	field **uint,
	update objectUpdate[T],
	log *logr.Logger,
) error {
	// require the runtime before reading its certificates workload id
	if err := requireRuntime(runtime, "certificates component", true); err != nil {
		return err
	}

	// install the certificates component on eks and kind
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

	// store the certificates workload id on the attached API instance
	record := recordComponentID(r, attacher, field, update)

	// install the certificates component and sync its manifest after reconcile
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

// EnsureSecretsComponent attaches an API instance to the secrets component of the support services operator.
// The shared workload is named external-secrets-<runtime>. Providers other than eks and kind are left unchanged.
func EnsureSecretsComponent[T componentAttacher](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	attacher T,
	field **uint,
	update objectUpdate[T],
	provider *string,
	log *logr.Logger,
) error {
	// require the runtime before reading its secrets workload id
	if err := requireRuntime(runtime, "secrets component", true); err != nil {
		return err
	}

	// install the secrets component on eks and kind
	if !secretsProviderCreates(provider) {
		return nil
	}

	// install the secrets component and sync its manifest after reconcile
	return ensure(
		r,
		runtime,
		&runtime.SecretsComponentInstanceID,
		"secrets component",
		fmt.Sprintf("external-secrets-%s", *runtime.Name),
		secretsColumn,
		recordComponentID(r, attacher, field, update),
		nil,
		externalSecretsYaml,
		log,
	)
}

// EnsureDatabaseComponent reports that the database component of the support services operator has no manifest.
func EnsureDatabaseComponent[T componentAttacher](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	attacher T,
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

// ReleaseIngressComponent detaches an API instance from the ingress component of the support services operator.
// It deletes the shared workload when the runtime still stores that workload id.
func ReleaseIngressComponent[T componentAttacher](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	attacher T,
) error {
	// require the runtime before reading its ingress workload id
	if err := requireRuntime(runtime, "ingress component", false); err != nil {
		return err
	}

	return releaseComponent(r, runtime, &runtime.IngressComponentInstanceID, "ingress component", ingressColumn, attacher)
}

// ReleaseCertificatesComponent detaches an API instance from the certificates component of the support services operator.
// It deletes the shared workload when the runtime still stores that workload id.
func ReleaseCertificatesComponent[T componentAttacher](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	attacher T,
) error {
	// require the runtime before reading its certificates workload id
	if err := requireRuntime(runtime, "certificates component", false); err != nil {
		return err
	}

	return releaseComponent(r, runtime, &runtime.CertificatesComponentInstanceID, "certificates component", certificatesColumn, attacher)
}

// ReleaseSecretsComponent detaches an API instance from the secrets component of the support services operator.
// It deletes the shared workload when the runtime still stores that workload id.
func ReleaseSecretsComponent[T componentAttacher](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	attacher T,
) error {
	// require the runtime before reading its secrets workload id
	if err := requireRuntime(runtime, "secrets component", false); err != nil {
		return err
	}

	return releaseComponent(r, runtime, &runtime.SecretsComponentInstanceID, "secrets component", secretsColumn, attacher)
}

// ReleaseDatabaseComponent detaches an API instance from the database component of the support services operator.
// It deletes the shared workload when the runtime still stores that workload id.
func ReleaseDatabaseComponent[T componentAttacher](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	attacher T,
) error {
	// require the runtime before reading its database workload id
	if err := requireRuntime(runtime, "database component", false); err != nil {
		return err
	}

	return releaseComponent(r, runtime, &runtime.DatabaseComponentInstanceID, "database component", databaseColumn, attacher)
}

// ensure installs one support services operator component when the runtime stores no workload id for it.
// It syncs the component manifest after that workload reconciles.
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
	// require the runtime id and name before naming the workload
	if runtime == nil || runtime.ID == nil || runtime.Name == nil {
		return fmt.Errorf("%s runtime is missing", label)
	}

	// sync the component manifest once the component workload has reconciled
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

	// store the workload id on the runtime and on the attached API instance
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

// recordComponentID stores the component workload id on the attached API instance.
// A nil field skips the store, and a nil update skips the save.
func recordComponentID[T any](
	r *controller.Reconciler,
	attacher T,
	field **uint,
	update objectUpdate[T],
) func(id *uint) error {
	return func(id *uint) error {
		// skip an absent field or one that already holds this workload id
		if field == nil {
			return nil
		}
		if *field != nil && id != nil && **field == *id {
			return nil
		}

		// store the workload id and save the attached API instance
		*field = id
		if update == nil {
			return nil
		}
		if _, err := update(r.APIClient, r.APIServer, attacher); err != nil {
			return fmt.Errorf("failed to record component workload: %w", err)
		}
		return nil
	}
}

// attacherManifest returns the manifest edit when the attached API instance implements one.
// An instance that keeps the stock component manifest returns nil.
func attacherManifest(attacher any) manifestEditor {
	editor, _ := attacher.(manifestEditor)
	return editor
}

// componentDocument returns the edited component manifest, or the stock manifest when the instance does not edit it.
func componentDocument(editor manifestEditor, manifest func() (string, error)) (string, error) {
	// return the stock component manifest when the instance adds nothing
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

// releaseComponent drops the attached API instance's requires row for the component workload.
// It deletes that workload when the runtime still stores its id.
func releaseComponent[T componentAttacher](
	r *controller.Reconciler,
	runtime *api.KubernetesRuntimeInstance,
	slot **uint,
	label string,
	column string,
	attacher T,
) error {
	// require the runtime before reading its component workload id
	if runtime == nil || runtime.ID == nil {
		return fmt.Errorf("%s runtime is missing", label)
	}

	return release(
		r,
		*runtime.ID,
		slot,
		label,
		column,
		attacher.GetFullyQualifiedType(),
		attacher.ObjectID(),
	)
}

// clearAttacherClaim drops this API instance's requires row and clears the runtime's component workload id.
func clearAttacherClaim(
	r *controller.Reconciler,
	runtimeID uint,
	workloadID uint,
	column string,
	attachedType string,
	attachedID *uint,
) (bool, error) {
	// keep the workload when the runtime names a different workload id
	held, err := componentColumnHeld(r, runtimeID, workloadID, column)
	if err != nil || !held {
		return held, err
	}

	// drop this API instance's requires row before clearing the runtime id
	if attachedID != nil {
		if err := dropComponentClaim(r, attachedType, *attachedID, workloadID); err != nil {
			return false, err
		}
	}

	return nullComponentColumn(r, runtimeID, column)
}

// release deletes the component workload after the attached API instance's requires row is dropped.
func release(
	r *controller.Reconciler,
	runtimeID uint,
	slot **uint,
	label string,
	column string,
	attachedType string,
	attachedID *uint,
) error {
	// return when the runtime stores no component workload id
	if *slot == nil {
		return nil
	}
	workloadID := **slot

	// delete the workload when this API instance held that id
	cleared, err := clearAttacherClaim(
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

	// delete the workload, and restore the id when the delete fails
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
