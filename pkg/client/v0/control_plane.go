package v0

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

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

// ErrNoGenesisControlPlane is returned when the database has no genesis row.
var ErrNoGenesisControlPlane = errors.New("no local control plane instance")

// GetGenesisControlPlaneInstance fetches the genesis control instance
func GetGenesisControlPlaneInstance(apiClient *http.Client, apiAddr string) (*v0.ControlPlaneInstance, error) {
	controlPlaneInstances, err := GetControlPlaneInstancesByQueryString(apiClient, apiAddr, "genesis=true")
	if err != nil {
		return nil, fmt.Errorf("could not retrieve genesis control plane instance: %w", err)
	}

	switch {
	case len(*controlPlaneInstances) < 1:
		return &v0.ControlPlaneInstance{}, ErrNoGenesisControlPlane
	case len(*controlPlaneInstances) > 1:
		return &v0.ControlPlaneInstance{}, errors.New(fmt.Sprintf("more than one local control plane instance %d returned", len(*controlPlaneInstances)))
	}

	return &(*controlPlaneInstances)[0], nil
}

// refreshCopiedGenesis copies the serving genesis onto a restored row.
// The self row, definition, and runtime stay, and an empty value does not wipe.
func refreshCopiedGenesis(stored, current *v0.ControlPlaneInstance) (*v0.ControlPlaneInstance, bool) {
	// leave the replacement's own row alone
	if stored == nil ||
		current == nil ||
		(stored.IsSelf != nil && *stored.IsSelf) {
		return stored, false
	}

	// copy the name, then the image and certificates
	changed := copyControlPlaneIdentity(stored, current)
	changed = copyControlPlaneImage(stored, current) || changed
	return stored, changed
}

// copyControlPlaneIdentity copies the name, namespace, and API address.
func copyControlPlaneIdentity(stored, current *v0.ControlPlaneInstance) bool {
	// copy each field the release base already accepts
	changed := copyControlPlaneString(&stored.Name, current.Name)
	changed = copyControlPlaneString(&stored.Namespace, current.Namespace) || changed
	return copyControlPlaneString(&stored.ApiServerEndpoint, current.ApiServerEndpoint) || changed
}

// copyControlPlaneImage copies the image and the client certificates.
func copyControlPlaneImage(stored, current *v0.ControlPlaneInstance) bool {
	// copy each field a release-base api can refuse
	changed := copyControlPlaneString(&stored.ImageNamespace, current.ImageNamespace)
	changed = copyControlPlaneString(&stored.ImageTag, current.ImageTag) || changed
	changed = copyControlPlaneString(&stored.CACert, current.CACert) || changed
	changed = copyControlPlaneString(&stored.ClientCert, current.ClientCert) || changed
	return copyControlPlaneString(&stored.ClientKey, current.ClientKey) || changed
}

// RefreshRestoredGenesis writes the current genesis onto the restored copy.
// A database with no genesis row is left for the caller to create.
func RefreshRestoredGenesis(apiClient *http.Client, apiAddr string, current *v0.ControlPlaneInstance) error {
	// leave the restored row alone when the caller has no current genesis
	if current == nil {
		return nil
	}

	// read the genesis row the backup restored
	stored, err := GetGenesisControlPlaneInstance(apiClient, apiAddr)
	if err != nil {
		// leave a database with no genesis row for the caller to create
		if errors.Is(err, ErrNoGenesisControlPlane) {
			return nil
		}
		return fmt.Errorf("failed to get restored genesis control plane: %w", err)
	}

	// leave the replacement's own row alone
	if stored == nil || (stored.IsSelf != nil && *stored.IsSelf) {
		return nil
	}

	// write the name and address before the image
	if copyControlPlaneIdentity(stored, current) {
		if _, err := UpdateControlPlaneInstance(apiClient, apiAddr, stored); err != nil {
			return fmt.Errorf("failed to refresh restored genesis control plane: %w", err)
		}
	}

	// write the image and certificates when the child api accepts them
	if !copyControlPlaneImage(stored, current) {
		return nil
	}
	if _, err := UpdateControlPlaneInstance(apiClient, apiAddr, stored); err != nil {
		if releaseBaseRejectsGenesisImage(err) {
			return nil
		}
		return fmt.Errorf("failed to refresh restored genesis control plane: %w", err)
	}
	return nil
}

// releaseBaseRejectsGenesisImage reports a release-base API refusing the image or a certificate.
func releaseBaseRejectsGenesisImage(err error) bool {
	// match the column named in the bad request
	if err == nil {
		return false
	}
	for _, field := range []string{"ImageNamespace", "ImageTag", "CACert", "ClientCert", "ClientKey"} {
		if strings.Contains(err.Error(), "cannot update "+field) {
			return true
		}
	}
	return false
}

// copyControlPlaneString copies a non-empty input argument onto the stored field.
func copyControlPlaneString(dest **string, value *string) bool {
	// leave an empty current value in place
	next := ""
	if value != nil {
		next = *value
	}
	if next == "" {
		return false
	}
	current := ""
	if *dest != nil {
		current = **dest
	}
	if next == current {
		return false
	}
	copied := next
	*dest = &copied
	return true
}
