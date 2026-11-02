package gateway

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"github.com/threeport/threeport/pkg/supportservices"
)

const certificatesComponent = "certificates"

// gatewayComponentAttacher is the caller that records one component for one gateway.
// An empty component selects the ingress workload.
type gatewayComponentAttacher struct {
	reconciler *controller.Reconciler
	gateway    *v0.GatewayInstance
	component  string
}

// certificates reports whether this attacher claims the certificates component.
func (a *gatewayComponentAttacher) certificates() bool {
	// select the certificates workload
	return a.component == certificatesComponent
}

// GetReconciler returns the reconciler this attacher uses for API calls.
func (a *gatewayComponentAttacher) GetReconciler() *controller.Reconciler {
	return a.reconciler
}

// Install creates the gateway component for this runtime.
func (a *gatewayComponentAttacher) Install(
	runtime *v0.KubernetesRuntimeInstance,
	log *logr.Logger,
) (uint, error) {
	// build the component manifest
	name := "ingress"
	manifest, err := supportservices.IngressComponentYaml()
	if a.certificates() {
		name = certificatesComponent
		manifest, err = supportservices.CertificatesComponentYaml()
	}
	if err != nil {
		return 0, fmt.Errorf("failed to create %s component: %w", name, err)
	}

	// create the component workload
	return supportservices.CreateComponentWorkload(
		a.reconciler,
		runtime,
		fmt.Sprintf("%s-%s", name, *runtime.Name),
		fmt.Sprintf("---\n%s\n", manifest),
		log,
	)
}

// IsComponentReconciled reports whether the workload has reconciled.
// A missing row returns reconciled false and missing true.
func (a *gatewayComponentAttacher) IsComponentReconciled(id uint) (bool, bool, error) {
	return supportservices.ComponentReconciled(a.reconciler, id)
}

// SetCallerKubernetesWorkloadInstanceID stores the workload id on the gateway.
func (a *gatewayComponentAttacher) SetCallerKubernetesWorkloadInstanceID(id *uint) error {
	// skip when this gateway already records the workload
	if a.gateway == nil || a.gateway.ID == nil || id == nil {
		return nil
	}
	recorded := a.gateway.IngressComponentInstanceID
	if a.certificates() {
		recorded = a.gateway.CertificatesComponentInstanceID
	}
	if recorded != nil {
		return nil
	}

	// store the workload id
	if a.certificates() {
		a.gateway.CertificatesComponentInstanceID = id
	} else {
		a.gateway.IngressComponentInstanceID = id
	}
	_, err := client.UpdateGatewayInstance(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		a.gateway,
	)
	if err != nil {
		return fmt.Errorf("failed to record gateway component workload %d: %w", *id, err)
	}
	return nil
}

// ClaimRuntimeKubernetesWorkloadInstanceID writes the runtime slot.
func (a *gatewayComponentAttacher) ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) error {
	// claim the certificates or ingress column
	column := supportservices.IngressColumn
	if a.certificates() {
		column = supportservices.CertificatesColumn
	}
	return supportservices.ClaimComponentWorkload(a.reconciler, runtimeID, workloadID, column)
}

// ClearRuntimeKubernetesWorkloadInstanceID clears the runtime slot when no caller requires the workload.
func (a *gatewayComponentAttacher) ClearRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) (bool, error) {
	// clear only the column this gateway holds
	column := supportservices.IngressColumn
	if a.certificates() {
		column = supportservices.CertificatesColumn
	}
	held, err := supportservices.ComponentColumnHeld(a.reconciler, runtimeID, workloadID, column)
	if err != nil || !held {
		return held, err
	}

	// drop this gateway's requires row before the column clear
	if err := a.dropComponentClaim(workloadID); err != nil {
		return false, err
	}
	return supportservices.NullComponentColumn(a.reconciler, runtimeID, column)
}

// dropComponentClaim deletes this gateway's requires row for the workload.
func (a *gatewayComponentAttacher) dropComponentClaim(workloadID uint) error {
	// skip a gateway that has not been stored
	if a.gateway == nil || a.gateway.ID == nil {
		return nil
	}

	// find the requires row this gateway wrote
	query := url.Values{}
	query.Set("objecttype", new(v0.KubernetesWorkloadInstance).GetFullyQualifiedType())
	query.Set("objectid", strconv.FormatUint(uint64(workloadID), 10))
	query.Set("attachedobjecttype", a.gateway.GetFullyQualifiedType())
	query.Set("attachedobjectid", strconv.FormatUint(uint64(*a.gateway.ID), 10))
	query.Set("relationship", string(v0.RelationshipRequires))
	references, err := client.GetAttachedObjectReferencesByQueryString(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
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
			a.reconciler.APIClient,
			a.reconciler.APIServer,
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

// DeleteComponentKubernetesWorkloadInstance removes the workload instance.
func (a *gatewayComponentAttacher) DeleteComponentKubernetesWorkloadInstance(id uint) error {
	return supportservices.DeleteComponentWorkload(a.reconciler, id)
}
