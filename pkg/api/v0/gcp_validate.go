package v0

import (
	"fmt"

	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// BeforeDelete validates a delete request on a gcp provider
// deletion to ensure deletion is possible.  A GcpProvider may not
// be deleted if it has related GcpGkeKubernetesRuntimeInstances.
func (g *GcpProvider) BeforeDelete(tx *gorm.DB) error {
	var gcpGkeKubernetesRuntimeInstances []GcpGkeKubernetesRuntimeInstance
	if result := tx.Where(
		&GcpGkeKubernetesRuntimeInstance{GcpProviderID: g.ID},
	).Find(&gcpGkeKubernetesRuntimeInstances); result.Error != nil {
		return fmt.Errorf(
			"failed to query gcp gke kubernetes runtime instances for gcp provider %s",
			*g.Name,
		)
	}

	if len(gcpGkeKubernetesRuntimeInstances) > 0 {
		return util.NewBadRequestError(
			fmt.Sprintf(
				"gcp provider %s has related gcp gke kubernetes runtime instances - cannot be deleted",
				*g.Name,
			),
		)
	}
	return nil
}
