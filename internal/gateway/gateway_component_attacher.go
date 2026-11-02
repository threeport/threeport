package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
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
	manifest, err := getIngressComponentYaml()
	if a.certificates() {
		name = certificatesComponent
		manifest, err = getCertificatesComponentYaml()
	}
	if err != nil {
		return 0, fmt.Errorf("failed to create %s component: %w", name, err)
	}

	// create the component workload
	return createComponentWorkload(
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
	// read the workload the slot names
	workload, err := client.GetKubernetesWorkloadInstanceByID(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		id,
	)
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		return false, true, nil
	}
	if err != nil {
		return false, false, err
	}
	if workload.DeletionScheduled != nil {
		return false, false, fmt.Errorf(
			"gateway component workload %d is being deleted",
			id,
		)
	}
	if workload.Reconciled != nil && *workload.Reconciled {
		return true, false, nil
	}
	return false, false, nil
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
	// load the runtime and set the slot
	runtime, err := client.GetKubernetesRuntimeInstanceByID(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		runtimeID,
	)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance %d: %w", runtimeID, err)
	}
	if a.certificates() {
		runtime.CertificatesComponentInstanceID = util.Ptr(workloadID)
	} else {
		runtime.IngressComponentInstanceID = util.Ptr(workloadID)
	}
	_, err = client.UpdateKubernetesRuntimeInstance(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		runtime,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to claim gateway component slot on runtime %d: %w",
			runtimeID,
			err,
		)
	}
	return nil
}

// ClearRuntimeKubernetesWorkloadInstanceID clears the runtime slot when no caller requires the workload.
func (a *gatewayComponentAttacher) ClearRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) (bool, error) {
	// load the runtime
	runtime, err := client.GetKubernetesRuntimeInstanceByID(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		runtimeID,
	)
	if err != nil {
		return false, fmt.Errorf(
			"failed to get kubernetes runtime instance %d: %w",
			runtimeID,
			err,
		)
	}
	slot := runtime.IngressComponentInstanceID
	if a.certificates() {
		slot = runtime.CertificatesComponentInstanceID
	}
	if slot == nil || *slot != workloadID {
		return false, nil
	}

	// drop this gateway's requires row so the clear can see the other callers
	if err := a.dropComponentClaim(workloadID); err != nil {
		return false, err
	}

	// replace the slot with null
	body, err := gatewayComponentSlotNull(a, runtime)
	if err != nil {
		return false, err
	}
	_, err = client_lib.GetResponse(
		a.reconciler.APIClient,
		fmt.Sprintf("%s%s/%d", a.reconciler.APIServer, v0.PathKubernetesRuntimeInstances, runtimeID),
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
		"failed to clear gateway component slot on runtime %d: %w",
		runtimeID,
		err,
	)
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
	// treat a missing workload as already removed
	_, err := client.DeleteKubernetesWorkloadInstance(
		a.reconciler.APIClient,
		a.reconciler.APIServer,
		id,
	)
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil
	}
	return err
}

// gatewayComponentSlotNull encodes the runtime with the component slot set to null.
func gatewayComponentSlotNull(a *gatewayComponentAttacher, runtime *v0.KubernetesRuntimeInstance) ([]byte, error) {
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
	if a.certificates() {
		fields["CertificatesComponentInstanceID"] = nil
	} else {
		fields["IngressComponentInstanceID"] = nil
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal kubernetes runtime instance: %w", err)
	}
	return body, nil
}
