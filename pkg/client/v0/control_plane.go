package v0

import (
	"errors"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// ErrNoLocalControlPlane is returned when a control plane stores no local row.
var ErrNoLocalControlPlane = errors.New("no local control plane instance")

// GetControlPlaneInstancesByControlPlaneDefinitionID fetches control plane
// instances by control plane definition ID.
func GetControlPlaneInstancesByControlPlaneDefinitionID(apiClient *http.Client, apiAddr string, id uint) (*[]v0.ControlPlaneInstance, error) {
	controlPlaneInstances, err := GetControlPlaneInstancesByQueryString(apiClient, apiAddr, fmt.Sprintf("controlplanedefinitionid=%d", id))
	if err != nil {
		return nil, fmt.Errorf("could not retrieve control plane instances with definition id: %w", err)
	}

	return controlPlaneInstances, nil
}

// GetSelfControlPlaneInstance fetches the control plane instance that represents the control plane being run on
func GetSelfControlPlaneInstance(apiClient *http.Client, apiAddr string) (*v0.ControlPlaneInstance, error) {
	controlPlaneInstances, err := GetControlPlaneInstancesByQueryString(apiClient, apiAddr, "isself=true")
	if err != nil {
		return nil, fmt.Errorf("could not retrieve self control plane instance: %w", err)
	}

	switch {
	case len(*controlPlaneInstances) < 1:
		return &v0.ControlPlaneInstance{}, ErrNoLocalControlPlane
	case len(*controlPlaneInstances) > 1:
		return &v0.ControlPlaneInstance{}, errors.New(fmt.Sprintf("more than one local control plane instance %d returned", len(*controlPlaneInstances)))
	}

	return &(*controlPlaneInstances)[0], nil
}

// ReparentDetachedControlPlanes points a detached child at self.
// A parent of self and a copied genesis row stay as they are.
func ReparentDetachedControlPlanes(apiClient *http.Client, apiAddr string, self *v0.ControlPlaneInstance) error {
	// read every control plane in the restored database
	if self == nil || self.ID == nil {
		return fmt.Errorf("failed to reparent control planes: restored self has no id")
	}
	instances, err := GetControlPlaneInstances(apiClient, apiAddr)
	if err != nil {
		return fmt.Errorf("failed to get control plane instances: %w", err)
	}

	// point a detached child at self
	byID := map[uint]v0.ControlPlaneInstance{}
	for _, instance := range *instances {
		if instance.ID != nil {
			byID[*instance.ID] = instance
		}
	}
	var updateErrors []error
	for _, instance := range *instances {
		if !reparentDetached(byID, *self.ID, instance) {
			continue
		}
		name := ""
		if instance.Name != nil {
			name = *instance.Name
		}
		instance.ParentControlPlaneInstanceID = self.ID
		if _, err := UpdateControlPlaneInstance(apiClient, apiAddr, &instance); err != nil {
			updateErrors = append(updateErrors, fmt.Errorf("failed to reparent control plane %s: %w", name, err))
		}
	}
	return errors.Join(updateErrors...)
}

// reparentDetached reports whether a row should point at self.
func reparentDetached(byID map[uint]v0.ControlPlaneInstance, selfID uint, instance v0.ControlPlaneInstance) bool {
	// leave a row that already reaches self
	if instance.ID == nil || *instance.ID == selfID || parentChainReaches(byID, *instance.ID, selfID) {
		return false
	}

	// leave a parent of self and the copied genesis row
	if parentChainReaches(byID, selfID, *instance.ID) {
		return false
	}
	if instance.Genesis != nil && *instance.Genesis {
		return false
	}
	return true
}

// parentChainReaches reports whether start's parent chain reaches ancestor.
func parentChainReaches(byID map[uint]v0.ControlPlaneInstance, start uint, ancestor uint) bool {
	// walk parents until self or a missing row
	current, ok := byID[start]
	seen := map[uint]bool{}
	for i := 0; i < 16 && ok; i++ {
		if current.ID != nil && seen[*current.ID] {
			return false
		}
		if current.ID != nil {
			seen[*current.ID] = true
		}
		if current.ParentControlPlaneInstanceID == nil {
			return false
		}
		if *current.ParentControlPlaneInstanceID == ancestor {
			return true
		}
		current, ok = byID[*current.ParentControlPlaneInstanceID]
	}
	return false
}

// GetGenesisControlPlaneInstance fetches the genesis control instance
func GetGenesisControlPlaneInstance(apiClient *http.Client, apiAddr string) (*v0.ControlPlaneInstance, error) {
	controlPlaneInstances, err := GetControlPlaneInstancesByQueryString(apiClient, apiAddr, "genesis=true")
	if err != nil {
		return nil, fmt.Errorf("could not retrieve genesis control plane instance: %w", err)
	}

	switch {
	case len(*controlPlaneInstances) < 1:
		return &v0.ControlPlaneInstance{}, ErrNoLocalControlPlane
	case len(*controlPlaneInstances) > 1:
		return &v0.ControlPlaneInstance{}, errors.New(fmt.Sprintf("more than one local control plane instance %d returned", len(*controlPlaneInstances)))
	}

	return &(*controlPlaneInstances)[0], nil
}
