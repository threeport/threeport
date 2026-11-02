package fixture

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	api "github.com/threeport/threeport/internal/reconcilertest/pkg/api/v0"
	fixtureclient "github.com/threeport/threeport/internal/reconcilertest/pkg/client/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	coreclient "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"github.com/threeport/threeport/pkg/supportservices"
)

// reconcilerTestIngressAttacher is the caller for the ingress component on one instance.
// It records that workload on the instance and on the runtime.
type reconcilerTestIngressAttacher struct {
	reconciler *controller.Reconciler
	instance   *api.ReconcilerTestInstance
}

// IsComponentReconciled reports whether the workload has reconciled.
// A missing row returns reconciled false and missing true.
func (a *reconcilerTestIngressAttacher) IsComponentReconciled(id uint) (bool, bool, error) {
	return supportservices.IsComponentReconciled(a.reconciler, id)
}

// SetCallerKubernetesWorkloadInstanceID records the workload on this instance.
func (a *reconcilerTestIngressAttacher) SetCallerKubernetesWorkloadInstanceID(id *uint) error {
	// skip an instance that has not been stored
	if a.instance == nil || a.instance.ID == nil || id == nil {
		return nil
	}
	if a.instance.IngressComponentInstanceID != nil && *a.instance.IngressComponentInstanceID == *id {
		return nil
	}

	// record the workload on this instance
	a.instance.IngressComponentInstanceID = id
	_, err := fixtureclient.UpdateReconcilerTestInstance(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		a.instance,
	)
	if err != nil {
		return fmt.Errorf("failed to record component workload on reconciler test instance: %w", err)
	}
	return nil
}

// ClaimRuntimeKubernetesWorkloadInstanceID stores the workload id on the runtime.
func (a *reconcilerTestIngressAttacher) ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) error {
	return supportservices.ClaimComponentWorkload(a.reconciler, runtimeID, workloadID, supportservices.IngressColumn)
}

// ClearRuntimeKubernetesWorkloadInstanceID clears the slot when this workload still holds it.
// It returns false when another workload holds the slot.
func (a *reconcilerTestIngressAttacher) ClearRuntimeKubernetesWorkloadInstanceID(
	runtimeID uint,
	workloadID uint,
) (bool, error) {
	// leave a slot that names a different workload
	column := supportservices.IngressColumn
	held, err := supportservices.ComponentColumnHeld(a.reconciler, runtimeID, workloadID, column)
	if err != nil || !held {
		return held, err
	}

	// drop this instance's requires row before the column clear
	if err := dropReconcilerTestComponentClaim(a.reconciler, a.instance, workloadID); err != nil {
		return false, err
	}
	return supportservices.NullComponentColumn(a.reconciler, runtimeID, column)
}

// DeleteComponentKubernetesWorkloadInstance removes the workload instance.
func (a *reconcilerTestIngressAttacher) DeleteComponentKubernetesWorkloadInstance(id uint) error {
	return supportservices.DeleteComponentWorkload(a.reconciler, id)
}

// reconcilerTestCertificatesAttacher is the caller for the certificates component on one instance.
// It records that workload on the instance and on the runtime.
type reconcilerTestCertificatesAttacher struct {
	reconciler *controller.Reconciler
	instance   *api.ReconcilerTestInstance
}

// IsComponentReconciled reports whether the workload has reconciled.
// A missing row returns reconciled false and missing true.
func (a *reconcilerTestCertificatesAttacher) IsComponentReconciled(id uint) (bool, bool, error) {
	return supportservices.IsComponentReconciled(a.reconciler, id)
}

// SetCallerKubernetesWorkloadInstanceID records the workload on this instance.
func (a *reconcilerTestCertificatesAttacher) SetCallerKubernetesWorkloadInstanceID(id *uint) error {
	// skip an instance that has not been stored
	if a.instance == nil || a.instance.ID == nil || id == nil {
		return nil
	}
	if a.instance.CertificatesComponentInstanceID != nil && *a.instance.CertificatesComponentInstanceID == *id {
		return nil
	}

	// record the workload on this instance
	a.instance.CertificatesComponentInstanceID = id
	_, err := fixtureclient.UpdateReconcilerTestInstance(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		a.instance,
	)
	if err != nil {
		return fmt.Errorf("failed to record component workload on reconciler test instance: %w", err)
	}
	return nil
}

// ClaimRuntimeKubernetesWorkloadInstanceID stores the workload id on the runtime.
func (a *reconcilerTestCertificatesAttacher) ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) error {
	return supportservices.ClaimComponentWorkload(
		a.reconciler,
		runtimeID,
		workloadID,
		supportservices.CertificatesColumn,
	)
}

// ClearRuntimeKubernetesWorkloadInstanceID clears the slot when this workload still holds it.
// It returns false when another workload holds the slot.
func (a *reconcilerTestCertificatesAttacher) ClearRuntimeKubernetesWorkloadInstanceID(
	runtimeID uint,
	workloadID uint,
) (bool, error) {
	// leave a slot that names a different workload
	column := supportservices.CertificatesColumn
	held, err := supportservices.ComponentColumnHeld(a.reconciler, runtimeID, workloadID, column)
	if err != nil || !held {
		return held, err
	}

	// drop this instance's requires row before the column clear
	if err := dropReconcilerTestComponentClaim(a.reconciler, a.instance, workloadID); err != nil {
		return false, err
	}
	return supportservices.NullComponentColumn(a.reconciler, runtimeID, column)
}

// DeleteComponentKubernetesWorkloadInstance removes the workload instance.
func (a *reconcilerTestCertificatesAttacher) DeleteComponentKubernetesWorkloadInstance(id uint) error {
	return supportservices.DeleteComponentWorkload(a.reconciler, id)
}

// reconcilerTestSecretsAttacher is the caller for the secrets component on one instance.
// It records that workload on the instance and on the runtime.
type reconcilerTestSecretsAttacher struct {
	reconciler *controller.Reconciler
	instance   *api.ReconcilerTestInstance
}

// IsComponentReconciled reports whether the workload has reconciled.
// A missing row returns reconciled false and missing true.
func (a *reconcilerTestSecretsAttacher) IsComponentReconciled(id uint) (bool, bool, error) {
	return supportservices.IsComponentReconciled(a.reconciler, id)
}

// SetCallerKubernetesWorkloadInstanceID records the workload on this instance.
func (a *reconcilerTestSecretsAttacher) SetCallerKubernetesWorkloadInstanceID(id *uint) error {
	// skip an instance that has not been stored
	if a.instance == nil || a.instance.ID == nil || id == nil {
		return nil
	}
	if a.instance.SecretsComponentInstanceID != nil && *a.instance.SecretsComponentInstanceID == *id {
		return nil
	}

	// record the workload on this instance
	a.instance.SecretsComponentInstanceID = id
	_, err := fixtureclient.UpdateReconcilerTestInstance(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		a.instance,
	)
	if err != nil {
		return fmt.Errorf("failed to record component workload on reconciler test instance: %w", err)
	}
	return nil
}

// ClaimRuntimeKubernetesWorkloadInstanceID stores the workload id on the runtime.
func (a *reconcilerTestSecretsAttacher) ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) error {
	return supportservices.ClaimComponentWorkload(a.reconciler, runtimeID, workloadID, supportservices.SecretsColumn)
}

// ClearRuntimeKubernetesWorkloadInstanceID clears the slot when this workload still holds it.
// It returns false when another workload holds the slot.
func (a *reconcilerTestSecretsAttacher) ClearRuntimeKubernetesWorkloadInstanceID(
	runtimeID uint,
	workloadID uint,
) (bool, error) {
	// leave a slot that names a different workload
	column := supportservices.SecretsColumn
	held, err := supportservices.ComponentColumnHeld(a.reconciler, runtimeID, workloadID, column)
	if err != nil || !held {
		return held, err
	}

	// drop this instance's requires row before the column clear
	if err := dropReconcilerTestComponentClaim(a.reconciler, a.instance, workloadID); err != nil {
		return false, err
	}
	return supportservices.NullComponentColumn(a.reconciler, runtimeID, column)
}

// DeleteComponentKubernetesWorkloadInstance removes the workload instance.
func (a *reconcilerTestSecretsAttacher) DeleteComponentKubernetesWorkloadInstance(id uint) error {
	return supportservices.DeleteComponentWorkload(a.reconciler, id)
}

// reconcilerTestDatabaseAttacher is the caller for the database component on one instance.
// It records that workload on the instance and on the runtime.
type reconcilerTestDatabaseAttacher struct {
	reconciler *controller.Reconciler
	instance   *api.ReconcilerTestInstance
}

// IsComponentReconciled reports whether the workload has reconciled.
// A missing row returns reconciled false and missing true.
func (a *reconcilerTestDatabaseAttacher) IsComponentReconciled(id uint) (bool, bool, error) {
	return supportservices.IsComponentReconciled(a.reconciler, id)
}

// SetCallerKubernetesWorkloadInstanceID records the workload on this instance.
func (a *reconcilerTestDatabaseAttacher) SetCallerKubernetesWorkloadInstanceID(id *uint) error {
	// skip an instance that has not been stored
	if a.instance == nil || a.instance.ID == nil || id == nil {
		return nil
	}
	if a.instance.DatabaseComponentInstanceID != nil && *a.instance.DatabaseComponentInstanceID == *id {
		return nil
	}

	// record the workload on this instance
	a.instance.DatabaseComponentInstanceID = id
	_, err := fixtureclient.UpdateReconcilerTestInstance(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		a.instance,
	)
	if err != nil {
		return fmt.Errorf("failed to record component workload on reconciler test instance: %w", err)
	}
	return nil
}

// ClaimRuntimeKubernetesWorkloadInstanceID stores the workload id on the runtime.
func (a *reconcilerTestDatabaseAttacher) ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) error {
	return supportservices.ClaimComponentWorkload(a.reconciler, runtimeID, workloadID, supportservices.DatabaseColumn)
}

// ClearRuntimeKubernetesWorkloadInstanceID clears the slot when this workload still holds it.
// It returns false when another workload holds the slot.
func (a *reconcilerTestDatabaseAttacher) ClearRuntimeKubernetesWorkloadInstanceID(
	runtimeID uint,
	workloadID uint,
) (bool, error) {
	// leave a slot that names a different workload
	column := supportservices.DatabaseColumn
	held, err := supportservices.ComponentColumnHeld(a.reconciler, runtimeID, workloadID, column)
	if err != nil || !held {
		return held, err
	}

	// drop this instance's requires row before the column clear
	if err := dropReconcilerTestComponentClaim(a.reconciler, a.instance, workloadID); err != nil {
		return false, err
	}
	return supportservices.NullComponentColumn(a.reconciler, runtimeID, column)
}

// DeleteComponentKubernetesWorkloadInstance removes the workload instance.
func (a *reconcilerTestDatabaseAttacher) DeleteComponentKubernetesWorkloadInstance(id uint) error {
	return supportservices.DeleteComponentWorkload(a.reconciler, id)
}

// dropReconcilerTestComponentClaim deletes this instance's requires row for the workload.
func dropReconcilerTestComponentClaim(
	reconciler *controller.Reconciler,
	instance *api.ReconcilerTestInstance,
	workloadID uint,
) error {
	// skip an instance that has not been stored
	if instance == nil || instance.ID == nil {
		return nil
	}

	// find the requires row this instance wrote
	query := url.Values{}
	query.Set("objecttype", new(v0.KubernetesWorkloadInstance).GetFullyQualifiedType())
	query.Set("objectid", strconv.FormatUint(uint64(workloadID), 10))
	query.Set("attachedobjecttype", instance.GetFullyQualifiedType())
	query.Set("attachedobjectid", strconv.FormatUint(uint64(*instance.ID), 10))
	query.Set("relationship", string(v0.RelationshipRequires))
	references, err := coreclient.GetAttachedObjectReferencesByQueryString(
		reconciler.APIClient,
		reconciler.APIServer,
		query.Encode(),
	)
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf(
			"failed to get attached object references for workload %d: %w",
			workloadID,
			err,
		)
	}

	// delete each row so the slot clear can proceed
	for _, reference := range *references {
		if reference.ID == nil {
			continue
		}
		_, err := coreclient.DeleteAttachedObjectReference(
			reconciler.APIClient,
			reconciler.APIServer,
			*reference.ID,
		)
		if err != nil && !errors.Is(err, client_lib.ErrObjectNotFound) {
			return fmt.Errorf(
				"failed to delete attached object reference %d: %w",
				*reference.ID,
				err,
			)
		}
	}
	return nil
}

var (
	_ supportservices.SupportServiceAttacher = (*reconcilerTestIngressAttacher)(nil)
	_ supportservices.SupportServiceAttacher = (*reconcilerTestCertificatesAttacher)(nil)
	_ supportservices.SupportServiceAttacher = (*reconcilerTestSecretsAttacher)(nil)
	_ supportservices.SupportServiceAttacher = (*reconcilerTestDatabaseAttacher)(nil)
)
