package secret

import (
	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// secretInstanceCreated reconciles state for a new secret
// instance.
func secretInstanceCreated(
	r *controller.Reconciler,
	secretInstance *v0.SecretInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// secretInstanceCreated reconciles state for a secret instance
// instance when it is changed.
func secretInstanceUpdated(
	r *controller.Reconciler,
	secretInstance *v0.SecretInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// secretInstanceCreated reconciles state for a secret instance
// instance when it is removed.
func secretInstanceDeleted(
	r *controller.Reconciler,
	secretInstance *v0.SecretInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}
