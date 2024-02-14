package terraform

import (
	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// terraformDefinitionCreated reconciles state for a new terraform definition.
// runtime definition.
func terraformDefinitionCreated(
	r *controller.Reconciler,
	terraformDefinition *v0.TerraformDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// terraformDefinitionCreated reconciles state for a terraform definition when
// it is changed.
func terraformDefinitionUpdated(
	r *controller.Reconciler,
	terraformDefinition *v0.TerraformDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// terraformDefinitionCreated reconciles state for a terraform definition when
// it is removed.
func terraformDefinitionDeleted(
	r *controller.Reconciler,
	terraformDefinition *v0.TerraformDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}
