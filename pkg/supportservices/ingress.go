// Package supportservices installs and removes the one workload a runtime shares across callers.
package supportservices

import (
	"errors"
	"fmt"

	api "github.com/threeport/threeport/pkg/api/v0"
)

// IngressAttacher is the object that uses the shared gateway controller.
type IngressAttacher interface {
	// IngressReady reports whether the workload has reconciled.
	// gone is true when that workload row does not exist.
	IngressReady(id uint) (ready bool, gone bool, err error)
	// SetIngressWorkloadID stores the workload id on the caller.
	SetIngressWorkloadID(id *uint) error
	// ClaimIngressSlot writes the runtime slot.
	ClaimIngressSlot(runtimeID uint, workloadID uint) error
	// ClearIngressSlot nils the runtime slot when no caller requires the workload.
	ClearIngressSlot(runtimeID uint, workloadID uint) (bool, error)
	// DeleteWorkload removes the workload instance.
	DeleteWorkload(id uint) error
}

// EnsureIngress installs the gateway controller when the runtime slot is empty.
func EnsureIngress(
	runtime *api.KubernetesRuntimeInstance,
	caller IngressAttacher,
	deploy func() (uint, error),
) error {
	// use the recorded workload when reconciliation has finished
	if runtime.GatewayControllerInstanceID != nil {
		workloadID := *runtime.GatewayControllerInstanceID
		ready, gone, err := caller.IngressReady(workloadID)
		if err != nil {
			return fmt.Errorf("failed to read gateway controller workload %d: %w", workloadID, err)
		}
		if ready {
			return caller.SetIngressWorkloadID(&workloadID)
		}
		if !gone {
			return fmt.Errorf("kubernetes workload instance %d is not reconciled", workloadID)
		}
	}

	// install, then claim; a lost claim deletes that workload
	workloadID, err := deploy()
	if err != nil {
		return fmt.Errorf("failed to deploy gateway controller for runtime %d: %w", *runtime.ID, err)
	}
	if err := caller.ClaimIngressSlot(*runtime.ID, workloadID); err != nil {
		_ = caller.DeleteWorkload(workloadID)
		return err
	}
	runtime.GatewayControllerInstanceID = &workloadID
	if err := caller.SetIngressWorkloadID(&workloadID); err != nil {
		return fmt.Errorf("failed to record gateway controller workload %d: %w", workloadID, err)
	}
	return fmt.Errorf("kubernetes workload instance %d is not reconciled", workloadID)
}

// ReleaseIngress removes the gateway controller when no caller still requires it.
// A failed delete restores the slot.
func ReleaseIngress(runtime *api.KubernetesRuntimeInstance, caller IngressAttacher) error {
	// nothing recorded
	if runtime.GatewayControllerInstanceID == nil {
		return nil
	}
	workloadID := *runtime.GatewayControllerInstanceID

	// clear the slot only when the compare finds no live requires row
	cleared, err := caller.ClearIngressSlot(*runtime.ID, workloadID)
	if err != nil {
		return fmt.Errorf("failed to clear gateway controller slot on runtime %d: %w", *runtime.ID, err)
	}
	if !cleared {
		return nil
	}

	// delete the workload, and put the slot back when that delete fails
	if err := caller.DeleteWorkload(workloadID); err != nil {
		if claimErr := caller.ClaimIngressSlot(*runtime.ID, workloadID); claimErr != nil {
			return fmt.Errorf(
				"failed to delete gateway controller workload %d and to restore the slot: %w",
				workloadID,
				errors.Join(err, claimErr),
			)
		}
		return fmt.Errorf("failed to delete gateway controller workload %d: %w", workloadID, err)
	}
	runtime.GatewayControllerInstanceID = nil
	return nil
}
