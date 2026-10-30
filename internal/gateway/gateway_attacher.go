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

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// gatewayAttacher records the shared controller on one gateway instance.
type gatewayAttacher struct {
	reconciler *controller.Reconciler
	gateway    *v0.GatewayInstance
}

// Ready reports whether the workload has reconciled.
func (a *gatewayAttacher) Ready(id uint) (bool, bool, error) {
	// read the workload the slot names
	workload, err := client.GetKubernetesWorkloadInstanceByID(a.reconciler.APIClient, a.reconciler.APIServer, id)
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		return false, true, nil
	}
	if err != nil {
		return false, false, err
	}
	if workload.DeletionScheduled != nil {
		return false, false, fmt.Errorf("gateway controller workload %d is being deleted", id)
	}
	if workload.Reconciled != nil && *workload.Reconciled {
		return true, false, nil
	}
	return false, false, nil
}

// SetWorkloadID stores the workload id on the gateway.
func (a *gatewayAttacher) SetWorkloadID(id *uint) error {
	// skip when this gateway already records the workload
	if a.gateway == nil || a.gateway.ID == nil || id == nil {
		return nil
	}
	if a.gateway.GatewayControllerInstanceID != nil {
		return nil
	}

	// store the workload id
	a.gateway.GatewayControllerInstanceID = id
	_, err := client.UpdateGatewayInstance(a.reconciler.APIClient, a.reconciler.APIServer, a.gateway)
	if err != nil {
		return fmt.Errorf("failed to record gateway controller workload %d: %w", *id, err)
	}
	return nil
}

// Claim writes the runtime slot.
func (a *gatewayAttacher) Claim(runtimeID uint, workloadID uint) error {
	// load the runtime and set the slot
	runtime, err := client.GetKubernetesRuntimeInstanceByID(a.reconciler.APIClient, a.reconciler.APIServer, runtimeID)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance %d: %w", runtimeID, err)
	}
	runtime.GatewayControllerInstanceID = &workloadID
	_, err = client.UpdateKubernetesRuntimeInstance(a.reconciler.APIClient, a.reconciler.APIServer, runtime)
	if err != nil {
		return fmt.Errorf("failed to claim gateway controller slot on runtime %d: %w", runtimeID, err)
	}
	return nil
}

// Clear nils the runtime slot when no caller requires the workload.
func (a *gatewayAttacher) Clear(runtimeID uint, workloadID uint) (bool, error) {
	// load the runtime
	runtime, err := client.GetKubernetesRuntimeInstanceByID(a.reconciler.APIClient, a.reconciler.APIServer, runtimeID)
	if err != nil {
		return false, fmt.Errorf("failed to get kubernetes runtime instance %d: %w", runtimeID, err)
	}
	if runtime.GatewayControllerInstanceID == nil || *runtime.GatewayControllerInstanceID != workloadID {
		return false, nil
	}

	// drop this gateway's requires row so the clear can see the other callers
	if err := a.dropIngressClaim(workloadID); err != nil {
		return false, err
	}

	// replace the slot with null
	body, err := gatewayControllerSlotNull(runtime)
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
	if errors.Is(err, client_lib.ErrBadRequest) && strings.Contains(err.Error(), v0.ErrMsgSupportServiceSlotHeld) {
		return false, nil
	}
	return false, fmt.Errorf("failed to clear gateway controller slot on runtime %d: %w", runtimeID, err)
}

// dropIngressClaim deletes this gateway's requires row for the workload.
func (a *gatewayAttacher) dropIngressClaim(workloadID uint) error {
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
		return fmt.Errorf("failed to get attached object references for workload %d: %w", workloadID, err)
	}

	// delete each row so the slot clear can proceed
	for _, reference := range *references {
		if reference.ID == nil {
			continue
		}
		_, err := client.DeleteAttachedObjectReference(a.reconciler.APIClient, a.reconciler.APIServer, *reference.ID)
		if err != nil && !errors.Is(err, client_lib.ErrObjectNotFound) {
			return fmt.Errorf("failed to delete attached object reference %d: %w", *reference.ID, err)
		}
	}
	return nil
}

// DeleteWorkload removes the workload instance.
func (a *gatewayAttacher) DeleteWorkload(id uint) error {
	// treat a missing workload as already removed
	_, err := client.DeleteKubernetesWorkloadInstance(a.reconciler.APIClient, a.reconciler.APIServer, id)
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil
	}
	return err
}

// gatewayControllerSlotNull encodes the runtime with the controller slot set to null.
func gatewayControllerSlotNull(runtime *v0.KubernetesRuntimeInstance) ([]byte, error) {
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
	fields["GatewayControllerInstanceID"] = nil
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal kubernetes runtime instance: %w", err)
	}
	return body, nil
}
