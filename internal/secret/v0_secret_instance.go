// generated by 'threeport-sdk gen' but will not be regenerated - intended for modification

package secret

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// v0SecretInstanceCreated performs reconciliation when a v0 SecretInstance
// has been created.
func v0SecretInstanceCreated(
	r *controller.Reconciler,
	secretInstance *v0.SecretInstance,
	log *logr.Logger,
) (int64, error) {
	// configure secret instance config
	c := &SecretInstanceConfig{
		r:              r,
		secretInstance: secretInstance,
		log:            log,
	}

	// get threeport objects
	if err := c.getThreeportObjects(); err != nil {
		return 0, fmt.Errorf("failed to get threeport objects: %w", err)
	}

	// validate threeport state
	if err := c.validateThreeportState(); err != nil {
		return 0, fmt.Errorf("failed to validate threeport state: %w", err)
	}

	// execute secret instance create operations
	if err := c.getSecretInstanceOperations().Create(); err != nil {
		return 0, fmt.Errorf("failed to execute secret instance create operations: %w", err)
	}

	return 0, nil
}

// v0SecretInstanceUpdated performs reconciliation when a v0 SecretInstance
// has been updated.
func v0SecretInstanceUpdated(
	r *controller.Reconciler,
	secretInstance *v0.SecretInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// v0SecretInstanceDeleted performs reconciliation when a v0 SecretInstance
// has been deleted.
func v0SecretInstanceDeleted(
	r *controller.Reconciler,
	secretInstance *v0.SecretInstance,
	log *logr.Logger,
) (int64, error) {
	// check that deletion is scheduled - if not there's a problem
	if secretInstance.DeletionScheduled == nil {
		return 0, errors.New("deletion notification receieved but not scheduled")
	}

	// check to see if confirmed - it should not be, but if so we should do no
	// more
	if secretInstance.DeletionConfirmed != nil {
		return 0, nil
	}

	// configure secret instance config
	c := &SecretInstanceConfig{
		r:              r,
		secretInstance: secretInstance,
		log:            log,
	}

	// get threeport objects
	if err := c.getThreeportObjects(); err != nil {
		if errors.Is(err, client_lib.ErrObjectNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get threeport objects: %w", err)
	}

	// execute secret instance delete operations
	if err := c.getSecretInstanceOperations().Delete(); err != nil {
		return 0, fmt.Errorf("failed to execute secret instance delete operations: %w", err)
	}

	return 0, nil
}
