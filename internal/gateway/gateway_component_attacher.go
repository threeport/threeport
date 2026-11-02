package gateway

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"github.com/threeport/threeport/pkg/supportservices"
)

// gatewayIngressAttacher is the caller for the ingress component on one gateway.
// It records that workload on the gateway and on the runtime.
type gatewayIngressAttacher struct {
	reconciler *controller.Reconciler
	gateway    *v0.GatewayInstance
}

// IsComponentReconciled reports whether the workload has reconciled.
// A missing row returns reconciled false and missing true.
func (a *gatewayIngressAttacher) IsComponentReconciled(id uint) (bool, bool, error) {
	return supportservices.IsComponentReconciled(a.reconciler, id)
}

// SetCallerKubernetesWorkloadInstanceID records the workload on this gateway.
func (a *gatewayIngressAttacher) SetCallerKubernetesWorkloadInstanceID(id *uint) error {
	// skip a gateway that has not been stored
	if a.gateway == nil || a.gateway.ID == nil || id == nil {
		return nil
	}
	if a.gateway.IngressComponentInstanceID != nil && *a.gateway.IngressComponentInstanceID == *id {
		return nil
	}

	// record the workload on this gateway
	a.gateway.IngressComponentInstanceID = id
	_, err := client.UpdateGatewayInstance(a.reconciler.APIClient, a.reconciler.APIServer, a.gateway)
	if err != nil {
		return fmt.Errorf("failed to record component workload on gateway instance: %w", err)
	}
	return nil
}

// ClaimRuntimeKubernetesWorkloadInstanceID stores the workload id on the runtime.
func (a *gatewayIngressAttacher) ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) error {
	return supportservices.ClaimComponentWorkload(a.reconciler, runtimeID, workloadID, supportservices.IngressColumn)
}

// ClearRuntimeKubernetesWorkloadInstanceID clears the slot when this workload still holds it.
// It returns false when another workload holds the slot.
func (a *gatewayIngressAttacher) ClearRuntimeKubernetesWorkloadInstanceID(
	runtimeID uint,
	workloadID uint,
) (bool, error) {
	// leave a slot that names a different workload
	column := supportservices.IngressColumn
	held, err := supportservices.ComponentColumnHeld(a.reconciler, runtimeID, workloadID, column)
	if err != nil || !held {
		return held, err
	}

	// drop this gateway's requires row before the column clear
	if err := dropGatewayComponentClaim(a.reconciler, a.gateway, workloadID); err != nil {
		return false, err
	}
	return supportservices.NullComponentColumn(a.reconciler, runtimeID, column)
}

// DeleteComponentKubernetesWorkloadInstance removes the workload instance.
func (a *gatewayIngressAttacher) DeleteComponentKubernetesWorkloadInstance(id uint) error {
	return supportservices.DeleteComponentWorkload(a.reconciler, id)
}

// gatewayCertificatesAttacher is the caller for the certificates component on one gateway.
// It records that workload on the gateway and on the runtime.
type gatewayCertificatesAttacher struct {
	reconciler *controller.Reconciler
	gateway    *v0.GatewayInstance
}

// IsComponentReconciled reports whether the workload has reconciled.
// A missing row returns reconciled false and missing true.
func (a *gatewayCertificatesAttacher) IsComponentReconciled(id uint) (bool, bool, error) {
	return supportservices.IsComponentReconciled(a.reconciler, id)
}

// SetCallerKubernetesWorkloadInstanceID records the workload on this gateway.
func (a *gatewayCertificatesAttacher) SetCallerKubernetesWorkloadInstanceID(id *uint) error {
	// skip a gateway that has not been stored
	if a.gateway == nil || a.gateway.ID == nil || id == nil {
		return nil
	}
	if a.gateway.CertificatesComponentInstanceID != nil && *a.gateway.CertificatesComponentInstanceID == *id {
		return nil
	}

	// record the workload on this gateway
	a.gateway.CertificatesComponentInstanceID = id
	_, err := client.UpdateGatewayInstance(a.reconciler.APIClient, a.reconciler.APIServer, a.gateway)
	if err != nil {
		return fmt.Errorf("failed to record component workload on gateway instance: %w", err)
	}
	return nil
}

// ClaimRuntimeKubernetesWorkloadInstanceID stores the workload id on the runtime.
func (a *gatewayCertificatesAttacher) ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) error {
	return supportservices.ClaimComponentWorkload(a.reconciler, runtimeID, workloadID, supportservices.CertificatesColumn)
}

// ClearRuntimeKubernetesWorkloadInstanceID clears the slot when this workload still holds it.
// It returns false when another workload holds the slot.
func (a *gatewayCertificatesAttacher) ClearRuntimeKubernetesWorkloadInstanceID(
	runtimeID uint,
	workloadID uint,
) (bool, error) {
	// leave a slot that names a different workload
	column := supportservices.CertificatesColumn
	held, err := supportservices.ComponentColumnHeld(a.reconciler, runtimeID, workloadID, column)
	if err != nil || !held {
		return held, err
	}

	// drop this gateway's requires row before the column clear
	if err := dropGatewayComponentClaim(a.reconciler, a.gateway, workloadID); err != nil {
		return false, err
	}
	return supportservices.NullComponentColumn(a.reconciler, runtimeID, column)
}

// DeleteComponentKubernetesWorkloadInstance removes the workload instance.
func (a *gatewayCertificatesAttacher) DeleteComponentKubernetesWorkloadInstance(id uint) error {
	return supportservices.DeleteComponentWorkload(a.reconciler, id)
}

// dropGatewayComponentClaim deletes this gateway's requires row for the workload.
func dropGatewayComponentClaim(
	reconciler *controller.Reconciler,
	gateway *v0.GatewayInstance,
	workloadID uint,
) error {
	// skip a gateway that has not been stored
	if gateway == nil || gateway.ID == nil {
		return nil
	}

	// find the requires row this gateway wrote
	query := url.Values{}
	query.Set("objecttype", new(v0.KubernetesWorkloadInstance).GetFullyQualifiedType())
	query.Set("objectid", strconv.FormatUint(uint64(workloadID), 10))
	query.Set("attachedobjecttype", gateway.GetFullyQualifiedType())
	query.Set("attachedobjectid", strconv.FormatUint(uint64(*gateway.ID), 10))
	query.Set("relationship", string(v0.RelationshipRequires))
	references, err := client.GetAttachedObjectReferencesByQueryString(
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
		_, err := client.DeleteAttachedObjectReference(
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
