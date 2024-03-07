package secret

import (
	"fmt"

	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// secretDefinitionCreated reconciles state for a new secret
// definition.
func secretDefinitionCreated(
	r *controller.Reconciler,
	secretDefinition *v0.SecretDefinition,
	log *logr.Logger,
) (int64, error) {
	// configure secret definition config
	secretDefinitionConfig := &SecretDefinitionConfig{
		r:                r,
		secretDefinition: secretDefinition,
		log:              log,
	}

	// push secret
	if err := secretDefinitionConfig.PushSecret(); err != nil {
		return 0, fmt.Errorf("failed to push secret: %w", err)
	}

	return 0, nil
}

// secretDefinitionCreated reconciles state for a secret
// definition when it is changed.
func secretDefinitionUpdated(
	r *controller.Reconciler,
	secretDefinition *v0.SecretDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// secretDefinitionCreated reconciles state for a secret
// definition when it is removed.
func secretDefinitionDeleted(
	r *controller.Reconciler,
	secretDefinition *v0.SecretDefinition,
	log *logr.Logger,
) (int64, error) {
	// configure secret definition config
	secretDefinitionConfig := &SecretDefinitionConfig{
		r:                r,
		secretDefinition: secretDefinition,
		log:              log,
	}

	// push secret
	if err := secretDefinitionConfig.DeleteSecret(); err != nil {
		return 0, fmt.Errorf("failed to delete secret: %w", err)
	}
	return 0, nil
}
