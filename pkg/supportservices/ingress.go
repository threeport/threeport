// Package supportservices installs and removes the one workload a runtime shares across callers.
package supportservices

import (
	"errors"
	"fmt"
)

// Attacher is the object that uses one shared workload.
type Attacher interface {
	// Ready reports whether the workload has reconciled.
	// gone is true when that workload row does not exist.
	Ready(id uint) (ready bool, gone bool, err error)
	// SetWorkloadID stores the workload id on the caller.
	SetWorkloadID(id *uint) error
	// Claim writes the runtime slot.
	Claim(runtimeID uint, workloadID uint) error
	// Clear nils the runtime slot when no caller requires the workload.
	Clear(runtimeID uint, workloadID uint) (bool, error)
	// DeleteWorkload removes the workload instance.
	DeleteWorkload(id uint) error
}

// Ensure installs the named workload when slot is empty.
// It stores the workload id in slot and returns an error until that workload is reconciled.
func Ensure(
	runtimeID uint,
	slot **uint,
	label string,
	caller Attacher,
	deploy func() (uint, error),
) error {
	// adopt the workload the slot already names
	if *slot != nil {
		ready, gone, err := caller.Ready(**slot)
		if err != nil {
			return fmt.Errorf("failed to read %s workload %d: %w", label, **slot, err)
		}
		if ready {
			return caller.SetWorkloadID(*slot)
		}
		if !gone {
			return fmt.Errorf("%s workload %d is not reconciled", label, **slot)
		}
	}

	// install the workload and claim the slot
	workloadID, err := deploy()
	if err != nil {
		return err
	}
	if err := caller.Claim(runtimeID, workloadID); err != nil {
		if deleteErr := caller.DeleteWorkload(workloadID); deleteErr != nil {
			return fmt.Errorf("failed to delete unclaimed %s workload %d: %w", label, workloadID, deleteErr)
		}
		return err
	}
	*slot = &workloadID
	if err := caller.SetWorkloadID(*slot); err != nil {
		return fmt.Errorf("failed to record %s workload %d: %w", label, workloadID, err)
	}
	return fmt.Errorf("%s workload %d is not reconciled", label, workloadID)
}

// Release removes the named workload when no caller still requires it.
// A failed delete restores the slot.
func Release(runtimeID uint, slot **uint, label string, caller Attacher) error {
	// nothing to remove when the slot is empty
	if *slot == nil {
		return nil
	}
	workloadID := **slot
	cleared, err := caller.Clear(runtimeID, workloadID)
	if err != nil {
		return fmt.Errorf("failed to clear %s slot: %w", label, err)
	}
	if !cleared {
		return nil
	}

	// delete the workload, and put the slot back when the delete fails
	if err := caller.DeleteWorkload(workloadID); err != nil {
		if claimErr := caller.Claim(runtimeID, workloadID); claimErr != nil {
			return fmt.Errorf("failed to delete %s workload %d: %w", label, workloadID, errors.Join(err, claimErr))
		}
		return fmt.Errorf("failed to delete %s workload %d: %w", label, workloadID, err)
	}
	*slot = nil
	return nil
}
