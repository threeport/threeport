package helmworkload

import (
	"errors"
	"fmt"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// WaitForHelmWorkloadInstanceReconciled waits for helm workload instance to be reconciled
func WaitForHelmWorkloadDefinitionReconciled(
	r *controller.Reconciler,
	id uint,
) error {
	// wait for helm workload instance to be reconciled
	if err := util.Retry(15, 1, func() error {
		var hwrd *v0.HelmWorkloadDefinition
		var err error
		if hwrd, err = client.GetHelmWorkloadDefinitionByID(
			r.APIClient,
			r.APIServer,
			id,
		); err != nil {
			return fmt.Errorf("failed to get helm workload definition: %w", err)
		}
		if !*hwrd.Reconciled {
			return fmt.Errorf("helm workload definition is not reconciled")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to wait for helm workload definition to be reconciled: %w", err)
	}
	return nil
}

// WaitForHelmWorkloadInstanceDeleted waits for helm workload instance to be deleted
func WaitForHelmWorkloadDefinitionDeleted(
	r *controller.Reconciler,
	id uint,
) error {
	// wait for helm workload definition to be deleted
	if err := util.Retry(15, 1, func() error {
		if _, err := client.GetHelmWorkloadDefinitionByID(
			r.APIClient,
			r.APIServer,
			id,
		); err != nil {
			if errors.Is(err, client.ErrObjectNotFound) {
				return nil
			}
			return fmt.Errorf("failed to get helm workload definition: %w", err)
		}
		return fmt.Errorf("helm workload definition still exists")
	}); err != nil {
		return fmt.Errorf("failed to wait for helm workload definition to be deleted: %w", err)
	}
	return nil
}

// WaitForHelmWorkloadInstanceReconciled waits for helm workload instance to be reconciled
func WaitForHelmWorkloadInstanceReconciled(
	r *controller.Reconciler,
	id uint,
) error {
	// wait for helm workload instance to be reconciled
	if err := util.Retry(60, 1, func() error {
		var hwri *v0.HelmWorkloadInstance
		var err error
		if hwri, err = client.GetHelmWorkloadInstanceByID(
			r.APIClient,
			r.APIServer,
			id,
		); err != nil {
			return fmt.Errorf("failed to get helm workload instance: %w", err)
		}
		if !*hwri.Reconciled {
			return fmt.Errorf("helm workload instance is not reconciled")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to wait for helm workload instance to be reconciled: %w", err)
	}
	return nil
}

// WaitForHelmWorkloadInstanceDeleted waits for helm workload instance to be deleted
func WaitForHelmWorkloadInstanceDeleted(
	r *controller.Reconciler,
	id uint,
) error {
	// wait for helm workload instance to be deleted
	if err := util.Retry(60, 1, func() error {
		if _, err := client.GetHelmWorkloadInstanceByID(
			r.APIClient,
			r.APIServer,
			id,
		); err != nil {
			if errors.Is(err, client.ErrObjectNotFound) {
				return nil
			}
			return fmt.Errorf("failed to get helm workload instance: %w", err)
		}
		return fmt.Errorf("workload instance still exists")
	}); err != nil {
		return fmt.Errorf("failed to wait for helm workload instance to be deleted: %w", err)
	}
	return nil
}
