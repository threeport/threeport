package v0

import (
	util "github.com/threeport/threeport/pkg/util/v0"
)

// rejectAbandonInfrastructureOnCreate refuses an instance created with
// AbandonInfrastructure already set. The flag lets a delete skip the
// controller's safeguards against orphaning cloud resources, and it stays set
// until changed, so an instance born with it set would never be protected. It
// is for an operator to set on an existing instance they are deleting.
func rejectAbandonInfrastructureOnCreate(abandon *bool) error {
	if abandon != nil && *abandon {
		return util.NewBadRequestError(
			"AbandonInfrastructure cannot be set when an instance is created: " +
				"it lets a delete skip the safeguards against orphaning cloud resources, " +
				"so set it only on an instance that is being deleted",
		)
	}

	return nil
}
