package cmd

import (
	"fmt"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// upgradeRollTarget returns the control plane whose deployments the upgrade rolls.
// An empty name rolls the connected control plane.
// A name rolls that descendant, and only when the connection is its parent or an ancestor.
func upgradeRollTarget(
	self *api_v0.ControlPlaneInstance,
	name string,
	byName func(string) (*api_v0.ControlPlaneInstance, error),
	byID func(uint) (*api_v0.ControlPlaneInstance, error),
) (*api_v0.ControlPlaneInstance, error) {
	// a connection to a child is refused before a name is considered
	if err := api_v0.ChildControlPlaneRequest(self); err != nil {
		return nil, err
	}
	if name == "" {
		return self, nil
	}

	// load the named control plane from the API this command is connected to
	target, err := byName(name)
	if err != nil {
		return nil, err
	}
	if !controlPlaneDescendsFrom(target, self, byID) {
		return nil, fmt.Errorf(
			"%s is not a control plane of %s",
			name,
			util.DerefString(self.Name),
		)
	}
	return target, nil
}

// controlPlaneDescendsFrom reports whether target's parent chain reaches ancestor.
func controlPlaneDescendsFrom(
	target *api_v0.ControlPlaneInstance,
	ancestor *api_v0.ControlPlaneInstance,
	byID func(uint) (*api_v0.ControlPlaneInstance, error),
) bool {
	if target == nil || ancestor == nil || ancestor.ID == nil {
		return false
	}
	// walk parents until the connected control plane or a root
	current := target
	for i := 0; i < 16; i++ {
		if current.ParentControlPlaneInstanceID == nil {
			return false
		}
		if *current.ParentControlPlaneInstanceID == *ancestor.ID {
			return true
		}
		parent, err := byID(*current.ParentControlPlaneInstanceID)
		if err != nil || parent == nil {
			return false
		}
		current = parent
	}
	return false
}
